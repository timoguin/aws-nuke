package resources

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/gotidy/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/ekristen/aws-nuke/v3/pkg/nuke"
)

func Test_ECSService_Properties(t *testing.T) {
	r := &ECSService{
		ServiceARN:             ptr.String("arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service"),
		ClusterARN:             ptr.String("arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"),
		ResourceManagementType: string(ecstypes.ResourceManagementTypeCustomer),
		Tags: []ecstypes.Tag{
			{
				Key:   ptr.String("Environment"),
				Value: ptr.String("test"),
			},
			{
				Key:   ptr.String("Project"),
				Value: ptr.String("aws-nuke"),
			},
		},
	}

	properties := r.Properties()

	assert.Equal(t, "arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service", properties.Get("ServiceARN"))
	assert.Equal(t, "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster", properties.Get("ClusterARN"))
	assert.Equal(t, "CUSTOMER", properties.Get("ResourceManagementType"))
	assert.Equal(t, "test", properties.Get("tag:Environment"))
	assert.Equal(t, "aws-nuke", properties.Get("tag:Project"))
}

func Test_ECSService_String(t *testing.T) {
	r := &ECSService{
		ServiceARN: ptr.String("arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service"),
		ClusterARN: ptr.String("arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"),
	}

	expected := "arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service -> " +
		"arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
	assert.Equal(t, expected, r.String())
}

func Test_ECSService_Remove(t *testing.T) {
	mockSvc := new(mockECSServiceClient)
	serviceARN := ptr.String("arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service")
	clusterARN := ptr.String("arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster")

	mockSvc.On("DeleteService", mock.Anything, &ecs.DeleteServiceInput{
		Cluster: clusterARN,
		Service: serviceARN,
		Force:   aws.Bool(true),
	}).Return(&ecs.DeleteServiceOutput{}, nil)

	r := &ECSService{
		svc:                    mockSvc,
		ServiceARN:             serviceARN,
		ClusterARN:             clusterARN,
		ResourceManagementType: string(ecstypes.ResourceManagementTypeCustomer),
	}

	err := r.Remove(context.TODO())

	assert.NoError(t, err)
	mockSvc.AssertExpectations(t)
}

func Test_ECSService_RemoveExpressGatewayService(t *testing.T) {
	mockSvc := new(mockECSServiceClient)
	serviceARN := ptr.String("arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service")
	clusterARN := ptr.String("arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster")

	mockSvc.On("DeleteExpressGatewayService", mock.Anything, &ecs.DeleteExpressGatewayServiceInput{
		ServiceArn: serviceARN,
	}).Return(&ecs.DeleteExpressGatewayServiceOutput{}, nil)

	r := &ECSService{
		svc:                    mockSvc,
		ServiceARN:             serviceARN,
		ClusterARN:             clusterARN,
		ResourceManagementType: string(ecstypes.ResourceManagementTypeEcs),
	}

	err := r.Remove(context.TODO())

	assert.NoError(t, err)
	mockSvc.AssertExpectations(t)
}

func Test_ECSService_List(t *testing.T) {
	mockSvc := new(mockECSServiceClient)
	clusterARN := "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
	serviceARN := "arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service"

	mockSvc.On("ListClusters", mock.Anything, mock.Anything).Return(&ecs.ListClustersOutput{
		ClusterArns: []string{clusterARN},
	}, nil)
	mockSvc.On("ListServices", mock.Anything, mock.Anything).Return(&ecs.ListServicesOutput{
		ServiceArns: []string{serviceARN},
	}, nil)
	mockSvc.On("DescribeServices", mock.Anything, &ecs.DescribeServicesInput{
		Cluster:  ptr.String(clusterARN),
		Services: []string{serviceARN},
		Include:  []ecstypes.ServiceField{ecstypes.ServiceFieldTags},
	}).Return(&ecs.DescribeServicesOutput{
		Services: []ecstypes.Service{
			{
				ServiceArn:             ptr.String(serviceARN),
				ClusterArn:             ptr.String(clusterARN),
				ResourceManagementType: ecstypes.ResourceManagementTypeEcs,
				Tags: []ecstypes.Tag{
					{Key: ptr.String("Environment"), Value: ptr.String("test")},
				},
			},
		},
	}, nil)

	lister := &ECSServiceLister{
		mockSvc: mockSvc,
	}

	resources, err := lister.List(context.TODO(), &nuke.ListerOpts{Config: &aws.Config{
		Region: "us-east-1",
	}})

	assert.NoError(t, err)
	assert.Len(t, resources, 1)

	r := resources[0].(*ECSService)
	assert.Equal(t, serviceARN, *r.ServiceARN)
	assert.Equal(t, clusterARN, *r.ClusterARN)
	assert.Equal(t, "ECS", r.ResourceManagementType)
	assert.Equal(t, "test", r.Properties().Get("tag:Environment"))
	mockSvc.AssertExpectations(t)
}

func Test_ECSService_ListDescribeServicesError(t *testing.T) {
	mockSvc := new(mockECSServiceClient)
	clusterARN := "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
	serviceARN := "arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service"

	mockSvc.On("ListClusters", mock.Anything, mock.Anything).Return(&ecs.ListClustersOutput{
		ClusterArns: []string{clusterARN},
	}, nil)
	mockSvc.On("ListServices", mock.Anything, mock.Anything).Return(&ecs.ListServicesOutput{
		ServiceArns: []string{serviceARN},
	}, nil)
	mockSvc.On("DescribeServices", mock.Anything, mock.Anything).Return(
		&ecs.DescribeServicesOutput{}, errors.New("describe failed"))

	lister := &ECSServiceLister{
		mockSvc: mockSvc,
	}

	resources, err := lister.List(context.TODO(), &nuke.ListerOpts{Config: &aws.Config{
		Region: "us-east-1",
	}})

	assert.Nil(t, resources)
	assert.EqualError(t, err, "describe failed")
	mockSvc.AssertExpectations(t)
}

func Test_ECSService_ListDescribeServicesFailure(t *testing.T) {
	mockSvc := new(mockECSServiceClient)
	clusterARN := "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"
	serviceARN := "arn:aws:ecs:us-east-1:123456789012:service/my-cluster/my-service"

	mockSvc.On("ListClusters", mock.Anything, mock.Anything).Return(&ecs.ListClustersOutput{
		ClusterArns: []string{clusterARN},
	}, nil)
	mockSvc.On("ListServices", mock.Anything, mock.Anything).Return(&ecs.ListServicesOutput{
		ServiceArns: []string{serviceARN},
	}, nil)
	mockSvc.On("DescribeServices", mock.Anything, mock.Anything).Return(&ecs.DescribeServicesOutput{
		Failures: []ecstypes.Failure{
			{
				Arn:    ptr.String(serviceARN),
				Reason: ptr.String("MISSING"),
			},
		},
	}, nil)

	lister := &ECSServiceLister{
		mockSvc: mockSvc,
	}

	resources, err := lister.List(context.TODO(), &nuke.ListerOpts{Config: &aws.Config{
		Region: "us-east-1",
	}})

	assert.Nil(t, resources)
	assert.ErrorContains(t, err, "failed to describe ECS services")
	mockSvc.AssertExpectations(t)
}

type mockECSServiceClient struct {
	mock.Mock
}

func (m *mockECSServiceClient) ListClusters(ctx context.Context, params *ecs.ListClustersInput,
	_ ...func(*ecs.Options)) (*ecs.ListClustersOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ecs.ListClustersOutput), args.Error(1)
}

func (m *mockECSServiceClient) ListServices(ctx context.Context, params *ecs.ListServicesInput,
	_ ...func(*ecs.Options)) (*ecs.ListServicesOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ecs.ListServicesOutput), args.Error(1)
}

func (m *mockECSServiceClient) DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput,
	_ ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ecs.DescribeServicesOutput), args.Error(1)
}

func (m *mockECSServiceClient) DeleteService(ctx context.Context, params *ecs.DeleteServiceInput,
	_ ...func(*ecs.Options)) (*ecs.DeleteServiceOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ecs.DeleteServiceOutput), args.Error(1)
}

func (m *mockECSServiceClient) DeleteExpressGatewayService(ctx context.Context,
	params *ecs.DeleteExpressGatewayServiceInput,
	_ ...func(*ecs.Options)) (*ecs.DeleteExpressGatewayServiceOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ecs.DeleteExpressGatewayServiceOutput), args.Error(1)
}
