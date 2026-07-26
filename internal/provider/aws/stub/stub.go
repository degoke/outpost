package stub

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// EC2 is an in-memory EC2 API fake for tests.
type EC2 struct {
	mu sync.Mutex

	nextSGID  atomic.Int32
	nextInst  atomic.Int32
	nextVol   atomic.Int32

	AMIID         string
	DefaultVPC    string
	SecurityGroups map[string]*ec2types.SecurityGroup
	Instances      map[string]*ec2types.Instance
	Volumes        map[string]bool
	DeletedSGs     []string
	DeletedVolumes []string
}

func New() *EC2 {
	s := &EC2{
		AMIID:          "ami-ubuntu-test",
		DefaultVPC:     "vpc-default",
		SecurityGroups: map[string]*ec2types.SecurityGroup{},
		Instances:      map[string]*ec2types.Instance{},
		Volumes:        map[string]bool{},
	}
	return s
}

func (s *EC2) DescribeImages(ctx context.Context, params *ec2.DescribeImagesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeImagesOutput, error) {
	ami := s.AMIID
	if ami == "" {
		ami = "ami-test"
	}
	return &ec2.DescribeImagesOutput{
		Images: []ec2types.Image{{
			ImageId:      aws.String(ami),
			CreationDate: aws.String("2026-01-01T00:00:00Z"),
		}},
	}, nil
}

func (s *EC2) DescribeVpcs(ctx context.Context, params *ec2.DescribeVpcsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error) {
	return &ec2.DescribeVpcsOutput{
		Vpcs: []ec2types.Vpc{{VpcId: aws.String(s.DefaultVPC)}},
	}, nil
}

func (s *EC2) CreateSecurityGroup(ctx context.Context, params *ec2.CreateSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := aws.ToString(params.GroupName)
	for _, sg := range s.SecurityGroups {
		if aws.ToString(sg.GroupName) == name && aws.ToString(sg.VpcId) == aws.ToString(params.VpcId) {
			return nil, fmt.Errorf("InvalidGroup.Duplicate: security group already exists")
		}
	}
	id := fmt.Sprintf("sg-%d", s.nextSGID.Add(1))
	s.SecurityGroups[id] = &ec2types.SecurityGroup{
		GroupId:   aws.String(id),
		GroupName: params.GroupName,
		VpcId:     params.VpcId,
	}
	return &ec2.CreateSecurityGroupOutput{GroupId: aws.String(id)}, nil
}

func (s *EC2) DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ec2types.SecurityGroup
	for _, sg := range s.SecurityGroups {
		if params.Filters != nil {
			match := true
			for _, f := range params.Filters {
				switch aws.ToString(f.Name) {
				case "group-name":
					if aws.ToString(sg.GroupName) != f.Values[0] {
						match = false
					}
				case "vpc-id":
					if aws.ToString(sg.VpcId) != f.Values[0] {
						match = false
					}
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, *sg)
	}
	return &ec2.DescribeSecurityGroupsOutput{SecurityGroups: out}, nil
}

func (s *EC2) AuthorizeSecurityGroupIngress(ctx context.Context, params *ec2.AuthorizeSecurityGroupIngressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error) {
	return &ec2.AuthorizeSecurityGroupIngressOutput{}, nil
}

func (s *EC2) AuthorizeSecurityGroupEgress(ctx context.Context, params *ec2.AuthorizeSecurityGroupEgressInput, optFns ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupEgressOutput, error) {
	return &ec2.AuthorizeSecurityGroupEgressOutput{}, nil
}

func (s *EC2) DeleteSecurityGroup(ctx context.Context, params *ec2.DeleteSecurityGroupInput, optFns ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := aws.ToString(params.GroupId)
	delete(s.SecurityGroups, id)
	s.DeletedSGs = append(s.DeletedSGs, id)
	return &ec2.DeleteSecurityGroupOutput{}, nil
}

func (s *EC2) RunInstances(ctx context.Context, params *ec2.RunInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := fmt.Sprintf("i-%d", s.nextInst.Add(1))
	volID := fmt.Sprintf("vol-%d", s.nextVol.Add(1))
	s.Volumes[volID] = true
	inst := ec2types.Instance{
		InstanceId:       aws.String(id),
		PublicDnsName:    aws.String(id + ".ec2.example.com"),
		PublicIpAddress:  aws.String("203.0.113.10"),
		InstanceType:     params.InstanceType,
		State:            &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning},
		BlockDeviceMappings: []ec2types.InstanceBlockDeviceMapping{{
			Ebs: &ec2types.EbsInstanceBlockDevice{VolumeId: aws.String(volID)},
		}},
	}
	s.Instances[id] = &inst
	return &ec2.RunInstancesOutput{Instances: []ec2types.Instance{inst}}, nil
}

func (s *EC2) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var reservations []ec2types.Reservation
	for _, id := range params.InstanceIds {
		if inst, ok := s.Instances[id]; ok {
			reservations = append(reservations, ec2types.Reservation{Instances: []ec2types.Instance{*inst}})
		}
	}
	if len(reservations) == 0 {
		return nil, fmt.Errorf("InvalidInstanceID.NotFound")
	}
	return &ec2.DescribeInstancesOutput{Reservations: reservations}, nil
}

func (s *EC2) TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range params.InstanceIds {
		if inst, ok := s.Instances[id]; ok {
			inst.State = &ec2types.InstanceState{Name: ec2types.InstanceStateNameTerminated}
		}
	}
	return &ec2.TerminateInstancesOutput{}, nil
}

func (s *EC2) StartInstances(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range params.InstanceIds {
		if inst, ok := s.Instances[id]; ok {
			inst.State = &ec2types.InstanceState{Name: ec2types.InstanceStateNameRunning}
		}
	}
	return &ec2.StartInstancesOutput{}, nil
}

func (s *EC2) StopInstances(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range params.InstanceIds {
		if inst, ok := s.Instances[id]; ok {
			inst.State = &ec2types.InstanceState{Name: ec2types.InstanceStateNameStopped}
		}
	}
	return &ec2.StopInstancesOutput{}, nil
}

func (s *EC2) RebootInstances(ctx context.Context, params *ec2.RebootInstancesInput, optFns ...func(*ec2.Options)) (*ec2.RebootInstancesOutput, error) {
	return &ec2.RebootInstancesOutput{}, nil
}

func (s *EC2) ModifyInstanceAttribute(ctx context.Context, params *ec2.ModifyInstanceAttributeInput, optFns ...func(*ec2.Options)) (*ec2.ModifyInstanceAttributeOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inst, ok := s.Instances[aws.ToString(params.InstanceId)]; ok && params.InstanceType != nil {
		inst.InstanceType = ec2types.InstanceType(aws.ToString(params.InstanceType.Value))
	}
	return &ec2.ModifyInstanceAttributeOutput{}, nil
}

func (s *EC2) DeleteVolume(ctx context.Context, params *ec2.DeleteVolumeInput, optFns ...func(*ec2.Options)) (*ec2.DeleteVolumeOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := aws.ToString(params.VolumeId)
	delete(s.Volumes, id)
	s.DeletedVolumes = append(s.DeletedVolumes, id)
	return &ec2.DeleteVolumeOutput{}, nil
}

func (s *EC2) HasSecurityGroup(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sg := range s.SecurityGroups {
		if aws.ToString(sg.GroupName) == name {
			return true
		}
	}
	return false
}

func (s *EC2) InstanceState(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inst, ok := s.Instances[id]; ok && inst.State != nil {
		return strings.ToLower(string(inst.State.Name))
	}
	return ""
}
