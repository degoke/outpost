const http = require("http");
const { Pool } = require("pg");
const { createClient } = require("redis");

const port = Number(process.env.PORT || 3000);
const databaseURL = process.env.DATABASE_URL;
const redisURL = process.env.REDIS_URL;

if (!databaseURL || !redisURL) {
  console.error("DATABASE_URL and REDIS_URL are required");
  process.exit(1);
}

async function main() {
  const pool = new Pool({ connectionString: databaseURL });
  const redis = createClient({ url: redisURL });

  redis.on("error", (err) => {
    console.error("redis error:", err);
  });

  await redis.connect();
  await pool.query(`
    CREATE TABLE IF NOT EXISTS visits (
      id SERIAL PRIMARY KEY,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
    )
  `);

  const server = http.createServer(async (req, res) => {
    try {
      if (req.method === "GET" && req.url === "/health") {
        await pool.query("SELECT 1");
        await redis.ping();
        return sendJSON(res, 200, { status: "ok", postgres: true, redis: true });
      }

      if (req.method === "POST" && req.url === "/visit") {
        const visits = await redis.incr("visits");
        const result = await pool.query(
          "INSERT INTO visits DEFAULT VALUES RETURNING id, created_at"
        );
        return sendJSON(res, 201, {
          visits,
          row: result.rows[0],
        });
      }

      if (req.method === "GET" && req.url === "/") {
        const visits = Number(await redis.get("visits")) || 0;
        const result = await pool.query("SELECT COUNT(*)::int AS total FROM visits");
        return sendJSON(res, 200, {
          message: "Node.js + Postgres + Redis example",
          redisVisits: visits,
          postgresRows: result.rows[0].total,
        });
      }

      sendJSON(res, 404, { error: "not found" });
    } catch (err) {
      console.error(err);
      sendJSON(res, 500, { error: "internal server error" });
    }
  });

  server.listen(port, () => {
    console.log(`listening on :${port}`);
  });

  const shutdown = async () => {
    server.close();
    await redis.quit();
    await pool.end();
    process.exit(0);
  };

  process.on("SIGINT", shutdown);
  process.on("SIGTERM", shutdown);
}

function sendJSON(res, status, body) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
