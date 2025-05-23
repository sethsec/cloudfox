package sdk

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	codeBuildTypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
)

type MockedCodeBuildClient struct {
}

func (m *MockedCodeBuildClient) ListProjects(ctx context.Context, input *codebuild.ListProjectsInput, options ...func(*codebuild.Options)) (*codebuild.ListProjectsOutput, error) {
	return &codebuild.ListProjectsOutput{
		Projects: []string{
			"project1",
			"project2",
		},
	}, nil
}

func (m *MockedCodeBuildClient) BatchGetProjects(ctx context.Context, input *codebuild.BatchGetProjectsInput, options ...func(*codebuild.Options)) (*codebuild.BatchGetProjectsOutput, error) {
	return &codebuild.BatchGetProjectsOutput{
		Projects: []codeBuildTypes.Project{
			{
				Name: aws.String("project1"),
			},
			{
				Name: aws.String("project2"),
			},
		},
	}, nil
}

func (m *MockedCodeBuildClient) GetResourcePolicy(ctx context.Context, input *codebuild.GetResourcePolicyInput, options ...func(*codebuild.Options)) (*codebuild.GetResourcePolicyOutput, error) {
	return &codebuild.GetResourcePolicyOutput{
		Policy: aws.String(`{
			"Version": "2012-10-17",
			"Statement": [
			  {
				"Effect": "Allow",
				"Action": "codebuild:BatchGetProjects",
				"Resource": "*",
				"Principal": {
					"AWS": "arn:aws:iam::123456789012:root"
				},
			  }
			]
		  }
		`),
	}, nil
}
