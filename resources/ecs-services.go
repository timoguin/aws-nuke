package resources

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"

	"github.com/ekristen/aws-nuke/v3/pkg/nuke"
)

const ECSServiceResource = "ECSService"

func init() {
	registry.Register(&registry.Registration{
		Name:     ECSServiceResource,
		Scope:    nuke.Account,
		Resource: &ECSService{},
		Lister:   &ECSServiceLister{},
	})
}

type ECSServiceClient interface {
	ListClusters(ctx context.Context, params *ecs.ListClustersInput,
		optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
	ListServices(ctx context.Context, params *ecs.ListServicesInput,
		optFns ...func(*ecs.Options)) (*ecs.ListServicesOutput, error)
	DescribeServices(ctx context.Context, params *ecs.DescribeServicesInput,
		optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	DeleteService(ctx context.Context, params *ecs.DeleteServiceInput,
		optFns ...func(*ecs.Options)) (*ecs.DeleteServiceOutput, error)
	DeleteExpressGatewayService(ctx context.Context, params *ecs.DeleteExpressGatewayServiceInput,
		optFns ...func(*ecs.Options)) (*ecs.DeleteExpressGatewayServiceOutput, error)
}

type ECSServiceLister struct {
	mockSvc ECSServiceClient
}

func (l *ECSServiceLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)

	var svc ECSServiceClient
	if l.mockSvc != nil {
		svc = l.mockSvc
	} else {
		svc = ecs.NewFromConfig(*opts.Config)
	}

	resources := make([]resource.Resource, 0)
	clusters := make([]string, 0)

	clusterParams := &ecs.ListClustersInput{
		MaxResults: aws.Int32(100),
	}

	// Iterate over clusters to ensure we dont presume its always default associations
	clusterPaginator := ecs.NewListClustersPaginator(svc, clusterParams)
	for clusterPaginator.HasMorePages() {
		output, err := clusterPaginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		clusters = append(clusters, output.ClusterArns...)
	}

	// Iterate over known clusters and discover their instances
	// to prevent assuming default is always used
	for _, clusterArn := range clusters {
		serviceParams := &ecs.ListServicesInput{
			Cluster:    aws.String(clusterArn),
			MaxResults: aws.Int32(10),
		}

		servicePaginator := ecs.NewListServicesPaginator(svc, serviceParams)
		for servicePaginator.HasMorePages() {
			output, err := servicePaginator.NextPage(ctx)
			if err != nil {
				return nil, err
			}

			for _, serviceArns := range chunkECSServiceARNs(output.ServiceArns, 10) {
				services, err := svc.DescribeServices(ctx, &ecs.DescribeServicesInput{
					Cluster:  aws.String(clusterArn),
					Services: serviceArns,
					Include:  []ecstypes.ServiceField{ecstypes.ServiceFieldTags},
				})
				if err != nil {
					return nil, err
				}
				if len(services.Failures) > 0 {
					return nil, fmt.Errorf("failed to describe ECS services: %v", services.Failures)
				}

				for _, service := range services.Services {
					resources = append(resources, &ECSService{
						svc:                    svc,
						ServiceARN:             service.ServiceArn,
						ClusterARN:             service.ClusterArn,
						ResourceManagementType: string(service.ResourceManagementType),
						Tags:                   service.Tags,
					})
				}
			}
		}
	}

	return resources, nil
}

type ECSService struct {
	svc                    ECSServiceClient
	ServiceARN             *string        `description:"The ARN of the ECS service"`
	ClusterARN             *string        `description:"The ARN of the ECS cluster"`
	ResourceManagementType string         `description:"Whether the ECS service is managed by the customer or by ECS"`
	Tags                   []ecstypes.Tag `description:"The tags associated with the service"`
}

func (f *ECSService) Properties() types.Properties {
	return types.NewPropertiesFromStruct(f)
}

func (f *ECSService) Remove(ctx context.Context) error {
	if f.ResourceManagementType == string(ecstypes.ResourceManagementTypeEcs) {
		_, err := f.svc.DeleteExpressGatewayService(ctx, &ecs.DeleteExpressGatewayServiceInput{
			ServiceArn: f.ServiceARN,
		})

		return err
	}

	_, err := f.svc.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: f.ClusterARN,
		Service: f.ServiceARN,
		Force:   aws.Bool(true),
	})

	return err
}

func (f *ECSService) String() string {
	return fmt.Sprintf("%s -> %s", *f.ServiceARN, *f.ClusterARN)
}

func chunkECSServiceARNs(serviceArns []string, size int) [][]string {
	if size <= 0 {
		return nil
	}

	chunks := make([][]string, 0, (len(serviceArns)+size-1)/size)
	for start := 0; start < len(serviceArns); start += size {
		end := start + size
		if end > len(serviceArns) {
			end = len(serviceArns)
		}

		chunks = append(chunks, serviceArns[start:end])
	}

	return chunks
}
