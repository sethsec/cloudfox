package cli

import (
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/service/kms"

	"github.com/BishopFox/cloudfox/aws"
	"github.com/BishopFox/cloudfox/aws/sdk"
	"github.com/BishopFox/cloudfox/internal"
	"github.com/BishopFox/cloudfox/internal/common"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"github.com/aws/aws-sdk-go-v2/service/apprunner"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/cloud9"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/codeartifact"
	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/codecommit"
	"github.com/aws/aws-sdk-go-v2/service/codedeploy"
	"github.com/aws/aws-sdk-go-v2/service/datapipeline"
	"github.com/aws/aws-sdk-go-v2/service/directoryservice"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticbeanstalk"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/emr"
	"github.com/aws/aws-sdk-go-v2/service/fsx"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/mq"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/ram"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go/ptr"
	"github.com/bishopfox/knownawsaccountslookup"
	"github.com/dominikbraun/graph"
	"github.com/fatih/color"
	"github.com/kyokomi/emoji"
	"github.com/spf13/cobra"
)

var (
	cyan             = color.New(color.FgCyan).SprintFunc()
	green            = color.New(color.FgGreen).SprintFunc()
	red              = color.New(color.FgRed).SprintFunc()
	magenta          = color.New(color.FgMagenta).SprintFunc()
	defaultOutputDir = ptr.ToString(internal.GetLogDirPath())

	AWSProfile          string
	AWSProfilesList     string
	AWSAllProfiles      bool
	AWSProfiles         []string
	AWSConfirm          bool
	AWSOutputType       string
	AWSTableCols        string
	PmapperDataBasePath string

	AWSOutputDirectory string
	AWSSkipAdminCheck  bool
	AWSWrapTable       bool
	AWSUseCache        bool
	AWSMFAToken        string

	Goroutines int
	Verbosity  int

	AWSCommands = &cobra.Command{
		Use:   "aws",
		Short: "See \"Available Commands\" for AWS Modules",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	AccessKeysFilter  string
	AccessKeysCommand = &cobra.Command{
		Use:     "access-keys",
		Aliases: []string{"accesskeys", "keys"},
		Short:   "Enumerate active access keys for all users",
		Long: "\nUse case examples:\n" +
			"Map active access keys:\n" +
			os.Args[0] + " aws access-keys --profile test_account" +
			os.Args[0] + " aws access-keys --filter access_key_id --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runAccessKeysCommand,
		PostRun: awsPostRun,
	}

	ApiGwCommand = &cobra.Command{
		Use:     "api-gw",
		Aliases: []string{"gw", "gateways", "api-gws"},
		Short:   "Enumerate API gateways. Get a loot file with formatted cURL requests.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws api-gw --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runApiGwCommand,
		PostRun: awsPostRun,
	}

	CheckBucketPolicies bool
	BucketsCommand      = &cobra.Command{
		Use:     "buckets",
		Aliases: []string{"bucket", "s3"},
		Short:   "Enumerate all of the buckets. Get loot file with s3 commands to list/download bucket contents",
		Long: "\nUse case examples:\n" +
			"List all buckets create a file with pre-populated aws s3 commands:\n" +
			os.Args[0] + " aws buckets --profile test_account",
		PreRun:  awsPreRun,
		Run:     runBucketsCommand,
		PostRun: awsPostRun,
	}

	CapeAdminOnly     bool
	CapeArnIgnoreList string
	CapeJobName       string
	CapeCommand       = &cobra.Command{
		Use:     "cape",
		Aliases: []string{"CAPE"},
		Short:   "Cross-Account Privilege Escalation Route finder. Needs to be run with multiple profiles using -l or -a flag. Needs pmapper data to be present",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws cape -l file_with_profile_names.txt --admin-only" +
			os.Args[0] + " aws cape -l file_with_profile_names.txt # This default mode shows all inbound paths but is very slow when there are many accounts)",
		PreRun:  awsPreRun,
		Run:     runCapeCommand,
		PostRun: awsPostRun,
	}

	CloudformationCommand = &cobra.Command{
		Use:     "cloudformation",
		Aliases: []string{"cf", "cfstacks", "stacks"},
		Short:   "Enumerate Cloudformation stacks. Get a loot file with stack details. Look for secrets.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws ecr --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runCloudformationCommand,
		PostRun: awsPostRun,
	}

	CodeBuildCommand = &cobra.Command{
		Use:   "codebuild",
		Short: "Enumerate CodeBuild projects.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws codebuild --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runCodeBuildCommand,
		PostRun: awsPostRun,
	}

	DatabasesCommand = &cobra.Command{
		Use:     "databases",
		Aliases: []string{"db", "rds", "redshift", "dbs"},
		Short:   "Enumerate databases. Get a loot file with connection strings.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws databases --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runDatabasesCommand,
		PostRun: awsPostRun,
	}

	ECRCommand = &cobra.Command{
		Use:     "ecr",
		Aliases: []string{"repos", "repo", "repositories"},
		Short:   "Enumerate the most recently pushed image URI from all repositories. Get a loot file with commands to pull images",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws ecr --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runECRCommand,
		PostRun: awsPostRun,
	}

	StoreSQSAccessPolicies bool
	SQSCommand             = &cobra.Command{
		Use:     "sqs",
		Aliases: []string{},
		Short:   "Enumerate SQS Queues.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws sqs --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runSQSCommand,
		PostRun: awsPostRun,
	}

	StoreSNSAccessPolicies bool
	SNSCommand             = &cobra.Command{
		Use:     "sns",
		Aliases: []string{},
		Short:   "Enumerate SNS Queues.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws sns --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runSNSCommand,
		PostRun: awsPostRun,
	}

	EKSCommand = &cobra.Command{
		Use:     "eks",
		Aliases: []string{"EKS", "clusters"},
		Short:   "Enumerate EKS clusters. Get a loot file with commands to authenticate with each cluster",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws --profile readonly_profile eks",
		PreRun:  awsPreRun,
		Run:     runEKSCommand,
		PostRun: awsPostRun,
	}

	EndpointsCommand = &cobra.Command{
		Use:     "endpoints",
		Aliases: []string{"endpoint"},
		Short:   "Enumerates endpoints from various services. Get a loot file with http endpoints to scan.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws endpoints --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runEndpointsCommand,
		PostRun: awsPostRun,
	}

	EnvsCommand = &cobra.Command{
		Use:     "env-vars",
		Aliases: []string{"envs", "envvars", "env"},
		Short:   "Enumerate the environment variables from multiple services that have them",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws env-vars --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runEnvsCommand,
		PostRun: awsPostRun,
	}

	FilesystemsCommand = &cobra.Command{
		Use:     "filesystems",
		Aliases: []string{"filesystem"},
		Short:   "Enumerate the EFS and FSx filesystems. Get a loot file with mount commands",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws filesystems --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runFilesystemsCommand,
		PostRun: awsPostRun,
	}

	SimulatorResource          string
	SimulatorAction            string
	SimulatorPrincipal         string
	IamSimulatorAdminCheckOnly bool
	IamSimulatorCommand        = &cobra.Command{
		Use:     "iam-simulator",
		Aliases: []string{"iamsimulator", "simulator"},
		Short:   "Wrapper around the AWS IAM Simulate Principal Policy command",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws iam-simulator --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runIamSimulatorCommand,
		PostRun: awsPostRun,
	}

	// This filter could be an instance ID or a TXT file with instance IDs separated by a new line.
	InstancesFilter                   string
	InstanceMapUserDataAttributesOnly bool
	InstancesCommand                  = &cobra.Command{
		Use:     "instances",
		Aliases: []string{"instance"},
		Short:   "Enumerate all instances along with assigned IPs, profiles, and user-data",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws instances --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runInstancesCommand,
		PostRun: awsPostRun,
	}

	ECSTasksCommand = &cobra.Command{
		Use:     "ecs-tasks",
		Aliases: []string{"ecs"},
		Short:   "Enumerate all ECS tasks along with assigned IPs and profiles",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws ecs-tasks --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runECSTasksCommand,
		PostRun: awsPostRun,
	}

	ElasticNetworkInterfacesCommand = &cobra.Command{
		Use:     "elastic-network-interfaces",
		Aliases: []string{"eni"},
		Short:   "Enumerate all elastic network interafces along with their private and public IPs and the VPC",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws elastic-network-interfaces --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runENICommand,
		PostRun: awsPostRun,
	}

	InventoryCommand = &cobra.Command{
		Use:   "inventory",
		Short: "Gain a rough understanding of size of the account and preferred regions",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws inventory --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runInventoryCommand,
		PostRun: awsPostRun,
	}

	LambdasCommand = &cobra.Command{
		Use:     "lambda",
		Aliases: []string{"lambdas", "functions"},
		Short:   "Enumerate lambdas.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws lambda --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runLambdasCommand,
		PostRun: awsPostRun,
	}

	NetworkPortsCommand = &cobra.Command{
		Use:     "network-ports",
		Aliases: []string{"ports", "networkports"},
		Short:   "Enumerate potentially accessible network ports.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws network-ports --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runNetworkPortsCommand,
		PostRun: awsPostRun,
	}

	OutboundAssumedRolesDays    int
	OutboundAssumedRolesCommand = &cobra.Command{
		Use:     "outbound-assumed-roles",
		Aliases: []string{"assumedroles", "assumeroles", "outboundassumedroles"},
		Short:   "Find the roles that have been assumed by principals in this account",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws outbound-assumed-roles --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runOutboundAssumedRolesCommand,
		PostRun: awsPostRun,
	}

	OrgsCommand = &cobra.Command{
		Use:     "orgs",
		Aliases: []string{"org", "organizations", "accounts", "account"},
		Short:   "Enumerate accounts in an organization",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws orgs --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runOrgsCommand,
		PostRun: awsPostRun,
	}

	PermissionsPrincipal string
	PermissionsCommand   = &cobra.Command{
		Use:     "permissions",
		Aliases: []string{"perms", "permission"},
		Short:   "Enumerate IAM permissions per principal",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws permissions --profile profile\n" +
			os.Args[0] + " aws permissions --profile profile --principal arn:aws:iam::111111111111:role/test123" +
			"\n\nAvailable Column Names:\n" +
			"Type, Name, Arn, Policy, Policy Name, Policy Arn, Effect, Action, Resource, Condition\n",

		PreRun:  awsPreRun,
		Run:     runPermissionsCommand,
		PostRun: awsPostRun,
	}

	PrincipalsCommand = &cobra.Command{
		Use:     "principals",
		Aliases: []string{"principal"},
		Short:   "Enumerate IAM users and Roles so you have the data at your fingertips",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws principals --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runPrincipalsCommand,
		PostRun: awsPostRun,
	}

	RAMCommand = &cobra.Command{
		Use:   "ram",
		Short: "Enumerate cross-account shared resources",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws ram --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runRAMCommand,
		PostRun: awsPostRun,
	}

	ResourceTrustsIncludeKms bool
	ResourceTrustsCommand    = &cobra.Command{
		Use:     "resource-trusts",
		Aliases: []string{"resourcetrusts", "resourcetrust"},
		Short:   "Enumerate all resource trusts",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws resource-trusts --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runResourceTrustsCommand,
		PostRun: awsPostRun,
	}

	// The filter is set to "all" when the flag "--filter" is not used
	RoleTrustFilter  string
	RoleTrustCommand = &cobra.Command{
		Use:     "role-trusts",
		Aliases: []string{"roletrusts", "role-trust"},
		Short:   "Enumerate all role trusts",
		Long: "\nUse case examples:\n" +
			"Map all role trusts for caller's account:\n" +
			os.Args[0] + " aws role-trusts\n",
		PreRun:  awsPreRun,
		Run:     runRoleTrustCommand,
		PostRun: awsPostRun,
	}

	Route53Command = &cobra.Command{
		Use:     "route53",
		Aliases: []string{"dns", "route", "routes"},
		Short:   "Enumerate all records from all zones managed by route53. Get a loot file with A records you can scan",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws route53 --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runRoute53Command,
		PostRun: awsPostRun,
	}

	SecretsCommand = &cobra.Command{
		Use:     "secrets",
		Aliases: []string{"secret"},
		Short:   "Enumerate secrets from secrets manager and SSM",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws secrets --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runSecretsCommand,
		PostRun: awsPostRun,
	}

	MaxResourcesPerRegion int
	TagsCommand           = &cobra.Command{
		Use:     "tags",
		Aliases: []string{"tag"},
		Short:   "Enumerate resources with tags.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws tags --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runTagsCommand,
		PostRun: awsPostRun,
	}
	PmapperCommand = &cobra.Command{

		Use:     "pmapper",
		Aliases: []string{"Pmapper", "pmapperParse"},
		Short:   "Looks for pmapper data for the account and builds a PrivEsc graph in golang if it exists.",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws ",
		PreRun:  awsPreRun,
		Run:     runPmapperCommand,
		PostRun: awsPostRun,
	}

	GraphCommand = &cobra.Command{
		Use:   "graph",
		Short: "INACTIVE (Use cape command instead) Graph the relationships between resources and insert into local Neo4j db",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws graph -l /path/to/profiles",
		PreRun:  awsPreRun,
		Run:     runGraphCommand,
		PostRun: awsPostRun,
	}

	WorkloadsCommand = &cobra.Command{
		Use:     "workloads",
		Short:   "Finds workloads with admin permissions or a path to admin permissions",
		Long:    "\nUse case examples:\n" + os.Args[0] + " aws workloads --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runWorkloadsCommand,
		PostRun: awsPostRun,
	}

	DirectoryServicesCommand = &cobra.Command{
		Use:     "ds",
		Short:   "Enumerate AWS-managed Active Directory instances and trusts",
		Long:    "\nUse case examples:\n" + os.Args[0] + " aws clouddirectory --profile readonly_profile",
		PreRun:  awsPreRun,
		Run:     runDirectoryServicesCommand,
		PostRun: awsPostRun,
	}

	AllChecksCommand = &cobra.Command{

		Use:     "all-checks",
		Aliases: []string{"allchecks", "all"},
		Short:   "Run all of the other checks (excluding outbound-assumed-roles)",
		Long: "\nUse case examples:\n" +
			os.Args[0] + " aws all-checks --profile readonly_profile", //TODO add examples? os.Args[0] + " aws all-checks --profiles profiles.txt, os.Args[0] + " aws all-checks --all-profiles""
		PreRun:  awsPreRun,
		Run:     runAllChecksCommand,
		PostRun: awsPostRun,
	}
)

func initAWSProfiles() {
	// Ensure only one profile setting is chosen
	if AWSProfile != "" && AWSProfilesList != "" || AWSProfile != "" && AWSAllProfiles || AWSProfilesList != "" && AWSAllProfiles {
		log.Fatalf("[-] Error specifying AWS profiles. Choose only one of -p/--profile, -a/--all-profiles, -l/--profiles-list")
	} else if AWSProfile != "" {
		AWSProfiles = append(AWSProfiles, AWSProfile)
	} else if AWSProfilesList != "" {
		// Written like so to enable testing while still being readable
		AWSProfiles = internal.GetSelectedAWSProfiles(AWSProfilesList)
	} else if AWSAllProfiles {
		AWSProfiles = internal.GetAllAWSProfiles(AWSConfirm)
	} else {
		AWSProfiles = append(AWSProfiles, "")
	}
}

type OrgAccounts struct {
	Organization *types.Organization
	Accounts     []types.Account
}

func awsPreRun(cmd *cobra.Command, args []string) {
	gob.Register(&types.Organization{})

	// if multiple profiles were used, ensure the management account is first
	// if AWSProfilesList != "" || AWSAllProfiles {
	// 	AWSProfiles = FindOrgMgmtAccountAndReorderAccounts(AWSProfiles, cmd.Root().Version, AWSMFAToken)
	// } else {

	// loop through every profile in AWSProfiles and run isCallerMgmtAccountPartofOrg.

	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		fmt.Printf("[%s][%s] AWS Caller Identity: %s\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile), *caller.Arn)
	}
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		if AWSUseCache {
			cacheDirectory := filepath.Join(AWSOutputDirectory, "cached-data", "aws", ptr.ToString(caller.Account))
			err = internal.LoadCacheFromGobFiles(cacheDirectory)
			if err != nil {
				if errors.Is(err, internal.ErrDirectoryDoesNotExist) {
					fmt.Printf("[%s][%s] No cache directory for %s. Skipping loading cached data.\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile), ptr.ToString(caller.Account))
				} else {
					fmt.Printf("[%s][%s] No cache data for %s. Error: %v\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile), ptr.ToString(caller.Account), err)
					// Possibly return/exit here, depending on your requirements.
				}
			} else {
				fmt.Printf("[%s][%s] Loaded cached AWS data for to %s\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile), ptr.ToString(caller.Account))
			}
		}
	}
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		orgModuleClient := aws.InitOrgsClient(*caller, profile, cmd.Root().Version, Goroutines, AWSMFAToken)
		isPartOfOrg := orgModuleClient.IsCallerAccountPartOfAnOrg()
		if isPartOfOrg {
			isMgmtAccount := orgModuleClient.IsManagementAccount(orgModuleClient.DescribeOrgOutput, ptr.ToString(caller.Account))
			if isMgmtAccount {
				fmt.Printf("[%s][%s] Account is part of an Organization and is the Management account\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile))
			} else {
				fmt.Printf("[%s][%s] Account is part of an Organization and is a child account. Management Account: %s\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile), ptr.ToString(orgModuleClient.DescribeOrgOutput.MasterAccountId))
			}
		} else {
			fmt.Printf("[%s][%s] Account is not part of an Organization\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile))
		}
		//}
	}
}

func awsPostRun(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		outputDirectory := filepath.Join(AWSOutputDirectory, "cached-data", "aws", ptr.ToString(caller.Account))
		err = internal.SaveCacheToGobFiles(outputDirectory, *caller.Account)
		if err != nil {
			log.Fatalf("failed to save cache: %v", err)
		}
		err = internal.SaveCacheToFiles(outputDirectory, *caller.Account)
		if err != nil {
			log.Fatalf("failed to save cache: %v", err)
		}

		fmt.Printf("[%s][%s] Cached AWS data written to %s\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), cyan(profile), outputDirectory)

	}
}

func FindOrgMgmtAccountAndReorderAccounts(AWSProfiles []string, version string) []string {
	//probably should create a map of mgmt accounts and child accounts
	//var mgmtAccounts map[string][]string
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, version, AWSMFAToken)
		if err != nil {
			continue
		}
		fmt.Printf("[%s] AWS Caller Identity: %s\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", version)), *caller.Arn)
		if AWSUseCache {
			cacheDirectory := filepath.Join(AWSOutputDirectory, "cached-data", "aws", ptr.ToString(caller.Account))
			err = internal.LoadCacheFromGobFiles(cacheDirectory)
			if err != nil {
				if errors.Is(err, internal.ErrDirectoryDoesNotExist) {
					fmt.Printf("[%s][%s] No cache directory for %s. Skipping loading cached data.\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", version)), cyan(profile), ptr.ToString(caller.Account))
				} else {
					fmt.Printf("[%s][%s] No cache data for %s. Error: %v\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", version)), cyan(profile), ptr.ToString(caller.Account), err)
					// Possibly return/exit here, depending on your requirements.
				}
			} else {
				fmt.Printf("[%s][%s] Loaded cached AWS data for to %s\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", version)), cyan(profile), ptr.ToString(caller.Account))
			}
		}
		orgModuleClient := aws.InitOrgsClient(*caller, profile, version, Goroutines, AWSMFAToken)
		orgModuleClient.DescribeOrgOutput, err = sdk.CachedOrganizationsDescribeOrganization(orgModuleClient.OrganizationsClient, ptr.ToString(caller.Account))
		if err != nil {
			continue
		}
		isMgmtAccount := orgModuleClient.IsManagementAccount(orgModuleClient.DescribeOrgOutput, ptr.ToString(caller.Account))
		if isMgmtAccount {
			mgmtAccount := ptr.ToString(caller.Account)
			fmt.Printf("[%s][%s] Found an Organization Management Account: %s\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", version)), cyan(profile), mgmtAccount)
			AWSProfiles = internal.ReorganizeAWSProfiles(AWSProfiles, profile)
		} else {
			// add each child account to the mgmtAccounts map which uses the mgmt account as the key
			//mgmtAccounts[ptr.ToString(orgModuleClient.DescribeOrgOutput.Organization.MasterAccountId)] = append(mgmtAccounts[ptr.ToString(orgModuleClient.DescribeOrgOutput.Organization.MasterAccountId)], ptr.ToString(caller.Account))
			continue
		}

	}
	return AWSProfiles
}

func runAccessKeysCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.AccessKeysModule{
			IAMClient:     iam.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintAccessKeys(AccessKeysFilter, AWSOutputDirectory, Verbosity)
	}
}

func runApiGwCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.ApiGwModule{
			APIGatewayClient:   apigateway.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			APIGatewayv2Client: apigatewayv2.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),

			Caller:     *caller,
			AWSRegions: internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile: profile,
			Goroutines: Goroutines,
			WrapTable:  AWSWrapTable,
		}
		m.PrintApiGws(AWSOutputDirectory, Verbosity)
	}
}

func runBucketsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		m := aws.BucketsModule{
			S3Client:            s3.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			WrapTable:           AWSWrapTable,
			CheckBucketPolicies: CheckBucketPolicies,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
		}
		m.PrintBuckets(AWSOutputDirectory, Verbosity)
	}

}

func runCloudformationCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.CloudformationModule{
			CloudFormationClient: cloudformation.NewFromConfig(AWSConfig),
			Caller:               *caller,
			AWSRegions:           internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:           profile,
			Goroutines:           Goroutines,
			WrapTable:            AWSWrapTable,
			AWSOutputType:        AWSOutputType,
			AWSTableCols:         AWSTableCols,
		}
		m.PrintCloudformationStacks(AWSOutputDirectory, Verbosity)
	}
}

func runCodeBuildCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.CodeBuildModule{
			CodeBuildClient:     codebuild.NewFromConfig(AWSConfig),
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.PrintCodeBuildProjects(AWSOutputDirectory, Verbosity)
	}
}

func runDatabasesCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		m := aws.DatabasesModule{
			RDSClient:      rds.NewFromConfig(AWSConfig),
			RedshiftClient: redshift.NewFromConfig(AWSConfig),
			DynamoDBClient: dynamodb.NewFromConfig(AWSConfig),
			Caller:         *caller,
			AWSRegions:     internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:     profile,
			Goroutines:     Goroutines,
			WrapTable:      AWSWrapTable,
			AWSOutputType:  AWSOutputType,
			AWSTableCols:   AWSTableCols,
		}
		m.PrintDatabases(AWSOutputDirectory, Verbosity)
	}
}

func runECRCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.ECRModule{
			ECRClient:     ecr.NewFromConfig(AWSConfig),
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintECR(AWSOutputDirectory, Verbosity)
	}
}

func runSQSCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.SQSModule{
			SQSClient: sqs.NewFromConfig(AWSConfig),

			StorePolicies: StoreSQSAccessPolicies,

			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintSQS(AWSOutputDirectory, Verbosity)
	}
}

func runSNSCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		cloudFoxSNSClient := aws.InitCloudFoxSNSClient(*caller, profile, cmd.Root().Version, Goroutines, AWSWrapTable, AWSMFAToken)
		cloudFoxSNSClient.PrintSNS(AWSOutputDirectory, Verbosity)
	}
}

func runEKSCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.EKSModule{
			IAMClient: iam.NewFromConfig(AWSConfig),
			EKSClient: eks.NewFromConfig(AWSConfig),

			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.EKS(AWSOutputDirectory, Verbosity)
	}
}

func runEndpointsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.EndpointsModule{
			APIGatewayClient:   apigateway.NewFromConfig(AWSConfig),
			APIGatewayv2Client: apigatewayv2.NewFromConfig(AWSConfig),
			AppRunnerClient:    apprunner.NewFromConfig(AWSConfig),
			CloudfrontClient:   cloudfront.NewFromConfig(AWSConfig),
			EKSClient:          eks.NewFromConfig(AWSConfig),
			ELBClient:          elasticloadbalancing.NewFromConfig(AWSConfig),
			ELBv2Client:        elasticloadbalancingv2.NewFromConfig(AWSConfig),
			GrafanaClient:      grafana.NewFromConfig(AWSConfig),
			LambdaClient:       lambda.NewFromConfig(AWSConfig),
			LightsailClient:    lightsail.NewFromConfig(AWSConfig),
			MQClient:           mq.NewFromConfig(AWSConfig),
			OpenSearchClient:   opensearch.NewFromConfig(AWSConfig),
			RDSClient:          rds.NewFromConfig(AWSConfig),
			RedshiftClient:     redshift.NewFromConfig(AWSConfig),
			S3Client:           s3.NewFromConfig(AWSConfig),

			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintEndpoints(AWSOutputDirectory, Verbosity)
	}
}

func runEnvsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.EnvsModule{
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,

			ECSClient:       ecs.NewFromConfig(AWSConfig),
			AppRunnerClient: apprunner.NewFromConfig(AWSConfig),
			LambdaClient:    lambda.NewFromConfig(AWSConfig),
			LightsailClient: lightsail.NewFromConfig(AWSConfig),
			SagemakerClient: sagemaker.NewFromConfig(AWSConfig),
		}
		m.PrintEnvs(AWSOutputDirectory, Verbosity)
	}
}

func runFilesystemsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		filesystems := aws.FilesystemsModule{
			EFSClient: efs.NewFromConfig(AWSConfig),
			FSxClient: fsx.NewFromConfig(AWSConfig),

			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		filesystems.PrintFilesystems(AWSOutputDirectory, Verbosity)
	}
}

func runGraphCommand(cmd *cobra.Command, args []string) {

	for _, profile := range AWSProfiles {
		//var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		//instantiate a permissions client and populate the permissions data
		fmt.Printf("[%s][%s] Getting account authorization details (GAAD) for account: %s\n", cyan("cape"), cyan(profile), ptr.ToString(caller.Account))

		PermissionsCommandClient := aws.InitPermissionsClient(*caller, profile, cmd.Root().Version, Goroutines, AWSMFAToken)
		PermissionsCommandClient.GetGAAD()
		PermissionsCommandClient.ParsePermissions("")
		common.PermissionRowsFromAllProfiles = append(common.PermissionRowsFromAllProfiles, PermissionsCommandClient.Rows...)
	}

	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		graphCommandClient := aws.GraphCommand{
			Caller:              *caller,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			AWSOutputDirectory:  AWSOutputDirectory,
			Verbosity:           Verbosity,
			AWSConfig:           AWSConfig,
			Version:             cmd.Root().Version,
			SkipAdminCheck:      AWSSkipAdminCheck,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		graphCommandClient.RunGraphCommand()
	}
}

func runCapeCommand(cmd *cobra.Command, args []string) {
	// map of all unique accountIDs and if they are included in the analysis or not
	//analyzedAccounts := make(map[string]bool)
	analyzedAccounts := make(map[string]aws.CapeJobInfo)

	GlobalGraph := graph.New(graph.StringHash, graph.Directed())
	//var PermissionRowsFromAllProfiles []common.PermissionsRow
	var GlobalPmapperData aws.PmapperModule
	var GlobalNodes []aws.Node

	vendors := knownawsaccountslookup.NewVendorMap()
	vendors.PopulateKnownAWSAccounts()

	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		_, err = internal.InitializeCloudFoxRunData(profile, cmd.Root().Version, AWSMFAToken, AWSOutputDirectory)
		if err != nil {
			continue
		}

		// add account number to analyzedAccounts map and set the value to true
		//analyzedAccounts[ptr.ToString(caller.Account)] = true
		analyzedAccounts[ptr.ToString(caller.Account)] = aws.CapeJobInfo{AccountID: ptr.ToString(caller.Account), Profile: profile, AnalyzedSuccessfully: true, AdminOnlyAnalysis: CapeAdminOnly, Source: "user"}

	}

	pmapperData := make(map[string]aws.PmapperModule)

	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		fmt.Printf("[%s][%s] Importing Pmapper data for: %s\n", cyan("cape"), cyan(profile), ptr.ToString(caller.Account))
		pmapperMod, pmapperError := aws.InitPmapperGraph(*caller, AWSProfile, Goroutines, PmapperDataBasePath)
		if pmapperError != nil {
			fmt.Println("Error importing pmapper data " + pmapperError.Error())
			analyzedAccounts[ptr.ToString(caller.Account)] = aws.CapeJobInfo{AnalyzedSuccessfully: false}
			// give the user the option to continue or not
			// if they choose to continue, we will skip the pmapper data and continue with the rest of the analysis
			// if they choose to not continue, we will exit the program
			fmt.Printf("Would you like to continue with the analysis without the pmapper data for profile %s? (y/n)", profile)
			var continueAnalysis string
			fmt.Scanln(&continueAnalysis)
			if continueAnalysis == "y" {
				continue
			} else {
				os.Exit(1)
			}
		}

		pmapperData[profile] = pmapperMod
	}
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		//Gather all Permissions data
		fmt.Printf("[%s][%s] Getting account authorization details (GAAD) for account: %s\n", cyan("cape"), cyan(profile), ptr.ToString(caller.Account))
		PermissionsCommandClient := aws.InitPermissionsClient(*caller, profile, cmd.Root().Version, Goroutines, AWSMFAToken)
		PermissionsCommandClient.GetGAAD()
		PermissionsCommandClient.ParsePermissions("")
		if PermissionsCommandClient.Rows != nil {
			common.PermissionRowsFromAllProfiles = append(common.PermissionRowsFromAllProfiles, PermissionsCommandClient.Rows...)
		} else {
			fmt.Println("Error gathering permissions for " + profile)
			//analyzedAccounts[ptr.ToString(caller.Account)] = false
			analyzedAccounts[ptr.ToString(caller.Account)] = aws.CapeJobInfo{AnalyzedSuccessfully: false}
		}

		// Gather all Pmapper data.
		//fmt.Printf("[%s][%s] Importing Pmapper for: %s\n", cyan("cape"), cyan(profile), ptr.ToString(caller.Account))

		// pmapperMod, pmapperError := aws.InitPmapperGraph(*caller, AWSProfile, Goroutines)
		// if pmapperError != nil {
		// 	fmt.Println("Error importing pmapper data " + pmapperError.Error())
		// 	//analyzedAccounts[ptr.ToString(caller.Account)] = false
		// 	analyzedAccounts[ptr.ToString(caller.Account)] = aws.CapeJobInfo{AnalyzedSuccessfully: false}
		// }

		pmapperMod := pmapperData[profile]

		// add pmapper nodes to GlobalNodes (which will also soon include iam roles and users)

		for _, node := range pmapperMod.Nodes {
			// add node to GlobalPmapperData
			//GlobalPmapperData.Nodes = append(GlobalPmapperData.Nodes, node)
			GlobalNodes = append(GlobalNodes, node)
		}
		fmt.Printf("[%s][%s] Added %d vertices from pmapper for %s\n", cyan("cape"), cyan(profile), len(pmapperMod.Nodes), ptr.ToString(caller.Account))

		// same for adding pmapper edges to GlobalPmapperData
		for _, edge := range pmapperMod.Edges {
			// add edge to GlobalPmapperData
			GlobalPmapperData.Edges = append(GlobalPmapperData.Edges, edge)
		}
		fmt.Printf("[%s][%s] Added %d edges from pmapper for %s\n", cyan("cape"), cyan(profile), len(pmapperMod.Edges), ptr.ToString(caller.Account))

		//Gather all role data so we can later process all of the role trusts and add external nodes not looked at by pmapper
		fmt.Printf("[%s][%s] Getting IAM roles for %s\n", cyan("cape"), cyan(profile), ptr.ToString(caller.Account))

		IAMCommandClient := aws.InitIAMClient(AWSConfig)
		ListRolesOutput, err := sdk.CachedIamListRoles(IAMCommandClient, ptr.ToString(caller.Account))
		if err != nil {
			internal.TxtLog.Error(err)
		}
		for _, role := range ListRolesOutput {
			//node := aws.ConvertIAMRoleToNode(role, vendors)
			node := aws.ConvertIAMRoleToNode(role, vendors, analyzedAccounts)

			// First insert the role itself into the Nodes slice
			GlobalNodes = append(GlobalNodes, node)
			// Then insert all of the vertices that are in the role's trust policy
			GlobalNodes = append(GlobalNodes, aws.FindVerticesInRoleTrust(node, vendors)...)

		}

		//Gather all user data
		// Currently, there is no need to parse groups and build group-user relationships because
		// the permissions command (and common.PermissionRowsFromAllProfiles above already has mapped/assigned group permissions to users within the group
		fmt.Printf("[%s][%s] Getting IAM users for %s\n", cyan("cape"), cyan(profile), ptr.ToString(caller.Account))

		ListUsersOutput, err := sdk.CachedIamListUsers(IAMCommandClient, ptr.ToString(caller.Account))
		if err != nil {
			internal.TxtLog.Error(err)
		}
		for _, user := range ListUsersOutput {
			GlobalNodes = append(GlobalNodes, aws.ConvertIAMUserToNode(user))

		}

	}

	//GlobalGraph := models.MakeAllVertices(GlobalRoles, GlobalPmapperData)

	// make vertices
	// you can't update vertices - so we need to make all of the vertices that are roles in the in-scope accounts
	// all at once to make sure they have the most information possible
	fmt.Printf("[%s] Making vertices for all profiles\n", cyan("cape"))

	// for _, role := range GlobalRoles {
	// 	role.MakeVertices(GlobalGraph)
	// }
	mergedNodes := aws.MergeNodes(GlobalNodes)
	for _, node := range mergedNodes {
		GlobalGraph.AddVertex(
			node.Arn,
			graph.VertexAttribute("Type", node.Type),
			graph.VertexAttribute("Name", node.Name),
			graph.VertexAttribute("VendorName", node.VendorName),
			graph.VertexAttribute("IsAdminString", node.IsAdminString),
			graph.VertexAttribute("CanPrivEscToAdminString", node.CanPrivEscToAdminString),
			graph.VertexAttribute("AccountID", node.AccountID),
		)
		// for every node, check to see if the accountId exists in the analyzedAccounts map. If it does not, add it to the map and set the value to false only if the node.VendorName is empty
		if _, ok := analyzedAccounts[node.AccountID]; !ok {
			if node.VendorName == "" {
				if node.AccountID != "" {
					analyzedAccounts[node.AccountID] = aws.CapeJobInfo{AccountID: node.AccountID, Profile: "", AnalyzedSuccessfully: false, AdminOnlyAnalysis: CapeAdminOnly, Source: "cloudfox"}
				}
			}
		}
	}

	// if the CapeArnIgnoreList arg is not empty, read the file and add the arns to the CapeArnIgnoreList
	if CapeArnIgnoreList != "" {
		// call ReadArnIgnoreListFile and add the arns to the CapeArnIgnoreList
		arnsToIgnore, err := aws.ReadArnIgnoreListFile(CapeArnIgnoreList)
		if err != nil {
			fmt.Println("Error reading the arn ignore list file: " + err.Error())
		}

		// remove nodes that are in the CapeArnIgnoreList from the graph
		for _, arn := range arnsToIgnore {
			GlobalGraph.RemoveVertex(arn)
		}
	}

	// make pmapper edges
	//you can update edges, so we can just merge attributes as needed
	// first we add the edges that already exist in pmapper, then later we will make more edges based on the cloudfox role trusts logic
	for _, edge := range GlobalPmapperData.Edges {
		err := GlobalGraph.AddEdge(
			edge.Source,
			edge.Destination,
			graph.EdgeAttribute(edge.ShortReason, edge.Reason),
		)
		if err != nil {
			if errors.Is(err, graph.ErrEdgeAlreadyExists) {
				// update theedge by copying the existing graph.Edge with attributes and add the new attributes
				//fmt.Println("Edge already exists")

				// get the existing edge
				existingEdge, _ := GlobalGraph.Edge(edge.Source, edge.Destination)
				// get the map of attributes
				existingProperties := existingEdge.Properties
				// add the new attributes to attributes map within the properties struct
				// Check if the Attributes map is initialized, if not, initialize it
				if existingProperties.Attributes == nil {
					existingProperties.Attributes = make(map[string]string)
				}

				// Add or update the attribute
				existingProperties.Attributes[edge.ShortReason] = edge.Reason
				GlobalGraph.UpdateEdge(
					edge.Source,
					edge.Destination,
					graph.EdgeAttributes(existingProperties.Attributes),
				)
			}
		}
	}

	//making edges
	// these are the cloudfox created edges mainly based on role trusts
	// at least for now, we don't need to make edges for users, groups, or anything else because pmapper already has all of the edges we need
	fmt.Printf("[%s] Making edges for all profiles\n", cyan("cape"))

	//fmt.Printf("[%s] Total vertices from pmapper and cape: %d \n", cyan("cape"), len(mergedNodes))

	for _, node := range mergedNodes {
		if node.Type == "Role" {
			node.MakeRoleEdges(GlobalGraph)
		}
	}

	// count the edges in the graph
	edges, _ := GlobalGraph.Edges()
	fmt.Printf("[%s] Total edges from pmapper and cape: %d \n", cyan("cape"), len(mergedNodes))
	fmt.Printf("[%s] Total edges from pmapper and cape: %d \n", cyan("cape"), len(edges))

	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		capeCommandClient := aws.CapeCommand{

			Caller:              *caller,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			AWSOutputDirectory:  AWSOutputDirectory,
			Verbosity:           Verbosity,
			AWSConfig:           AWSConfig,
			Version:             cmd.Root().Version,
			SkipAdminCheck:      AWSSkipAdminCheck,
			GlobalGraph:         GlobalGraph,
			PmapperDataBasePath: PmapperDataBasePath,
			AnalyzedAccounts:    analyzedAccounts,
			CapeAdminOnly:       CapeAdminOnly,
		}

		capeCommandClient.RunCapeCommand()

		// write a json file with job information to the output directory. Use the CapeJobName for the file name, and have the data include the list of AWSProfiles that were analyzed
		// this will be used by a TUI to match a job name to the list of accounts that were analyzed

		// if CapeJobName == "" {
		// 	// create random job name in the format of cape-timmefromepoch
		// 	CapeJobName = fmt.Sprintf("cape-%s", time.Now().Format("2006-01-02-15-04-05"))
		// }
		// filename := fmt.Sprintf("%s.json", CapeJobName)
		// filepath := filepath.Join(AWSOutputDirectory, "aws", "capeJobs")
		// err = os.MkdirAll(filepath, 0755)
		// if err != nil {
		// 	fmt.Println("Error creating directory: " + err.Error())
		// }
		// file, _ := os.Create(filepath + "/" + filename)
		// defer file.Close()
		// encoder := json.NewEncoder(file)
		// encoder.SetIndent("", "  ")
		// err = encoder.Encode(analyzedAccounts)
		// if err != nil {
		// 	fmt.Println("Error writing job data to file: " + err.Error())
		// } else {
		// 	fmt.Printf("[%s] Job output written to %s\n", cyan("cape"), file.Name())
		// 	fmt.Printf("[%s] %s\n\n", cyan("cape"), magenta("The results of the cape command are best viewed in the cape terminal user interface (TUI). Use the command below:"))
		// 	fmt.Printf("[%s] \tcloudfox aws -l %s cape tui\n\n", cyan("cape"), AWSProfilesList)
		// }

		// playing around with creating a graphviz file for image rendering.
		// the goal here is to be able to export this graph data to a format that can be easily imported in neo4j.
		// this is a work in progress and not yet complete

		// filename := fmt.Sprintf("./mygraph-%s-%s.gv", ptr.ToString(caller.Account), time.Now().Format("2006-01-02-15-04-05"))
		// file, _ := os.Create(filename)
		// _ = draw.DOT(GlobalGraph, file, draw.GraphAttribute(
		// 	"ranksep", "3",
		// ))
	}

	fmt.Printf("[%s] %s\n\n", cyan("cape"), magenta("The results of the cape command are best viewed in the cape terminal user interface (TUI). Use the command below:"))
	if CapeAdminOnly {
		fmt.Printf("\t\tcloudfox aws -l %s cape tui --admin-only\n\n", AWSProfilesList)
	} else {
		fmt.Printf("\t\tcloudfox aws -l %s cape tui\n\n", AWSProfilesList)
	}
}

func runCapeTUICommand(cmd *cobra.Command, args []string) {
	var capeOutputFileLocations []string
	for i, profile := range AWSProfiles {
		cloudfoxRunData, err := internal.InitializeCloudFoxRunData(profile, cmd.Root().Version, AWSMFAToken, AWSOutputDirectory)
		//caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		var fileName string
		if CapeAdminOnly {
			fileName = "inbound-privesc-paths-admin-targets-only.json"
		} else {
			fileName = "inbound-privesc-paths-all-targets.json"
		}
		capeOutputFileLocations = append(capeOutputFileLocations, filepath.Join(cloudfoxRunData.OutputLocation, "json", fileName))
		//check to see if file exists, and if it doesn't, remove the profile from the list of profiles to analyze and print a message to the console
		if _, err := os.Stat(filepath.Join(cloudfoxRunData.OutputLocation, "json", fileName)); os.IsNotExist(err) {
			fmt.Printf("[%s] Could not retrieve CAPE data for profile %s.\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)), profile)
			//remove the profile from the list of profiles to analyze
			if len(AWSProfiles) > 1 {
				AWSProfiles = append(AWSProfiles[:i], AWSProfiles[i+1:]...)
			} else {
				if CapeAdminOnly {
					fmt.Printf("[%s] Could not retrieve cape data. Did you run cape without the --admin-only flag? You'll need to run cape with --admin-only to use the tui with --admin-only\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)))
				} else {
					fmt.Printf("[%s] Did you run cape with the --admin-only flag? You'll need to run cape without --admin-only to use the tui without --admin-only\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)))
				}
				os.Exit(1)
			}
		}

	}

	if len(capeOutputFileLocations) == 0 {
		fmt.Printf("[%s] Could not retrieve CAPE data.\n", cyan(emoji.Sprintf(":fox:cloudfox v%s :fox:", cmd.Root().Version)))
		os.Exit(1)
	}
	aws.CapeTUI(capeOutputFileLocations)

}

func runIamSimulatorCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.IamSimulatorModule{
			IAMClient:                  iam.NewFromConfig(AWSConfig),
			Caller:                     *caller,
			AWSProfileProvided:         profile,
			Goroutines:                 Goroutines,
			WrapTable:                  AWSWrapTable,
			AWSOutputType:              AWSOutputType,
			AWSTableCols:               AWSTableCols,
			IamSimulatorAdminCheckOnly: IamSimulatorAdminCheckOnly,
		}
		m.PrintIamSimulator(SimulatorPrincipal, SimulatorAction, SimulatorResource, AWSOutputDirectory, Verbosity)
	}
}

func runInstancesCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.InstancesModule{
			EC2Client: ec2.NewFromConfig(AWSConfig),
			IAMClient: iam.NewFromConfig(AWSConfig),

			Caller:                 *caller,
			AWSRegions:             internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			UserDataAttributesOnly: InstanceMapUserDataAttributesOnly,
			AWSProfile:             profile,
			SkipAdminCheck:         AWSSkipAdminCheck,
			WrapTable:              AWSWrapTable,
			AWSOutputType:          AWSOutputType,
			AWSTableCols:           AWSTableCols,
			PmapperDataBasePath:    PmapperDataBasePath,
		}
		m.Instances(InstancesFilter, AWSOutputDirectory, Verbosity)
	}
}

func runInventoryCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.Inventory2Module{
			APIGatewayClient:       apigateway.NewFromConfig(AWSConfig),
			APIGatewayv2Client:     apigatewayv2.NewFromConfig(AWSConfig),
			AppRunnerClient:        apprunner.NewFromConfig(AWSConfig),
			AthenaClient:           athena.NewFromConfig(AWSConfig),
			Cloud9Client:           cloud9.NewFromConfig(AWSConfig),
			CloudFormationClient:   cloudformation.NewFromConfig(AWSConfig),
			CloudfrontClient:       cloudfront.NewFromConfig(AWSConfig),
			CodeArtifactClient:     codeartifact.NewFromConfig(AWSConfig),
			CodeBuildClient:        codebuild.NewFromConfig(AWSConfig),
			CodeCommitClient:       codecommit.NewFromConfig(AWSConfig),
			CodeDeployClient:       codedeploy.NewFromConfig(AWSConfig),
			DataPipelineClient:     datapipeline.NewFromConfig(AWSConfig),
			DynamoDBClient:         dynamodb.NewFromConfig(AWSConfig),
			EC2Client:              ec2.NewFromConfig(AWSConfig),
			ECSClient:              ecs.NewFromConfig(AWSConfig),
			ECRClient:              ecr.NewFromConfig(AWSConfig),
			EKSClient:              eks.NewFromConfig(AWSConfig),
			ELBClient:              elasticloadbalancing.NewFromConfig(AWSConfig),
			ELBv2Client:            elasticloadbalancingv2.NewFromConfig(AWSConfig),
			ElasticacheClient:      elasticache.NewFromConfig(AWSConfig),
			ElasticBeanstalkClient: elasticbeanstalk.NewFromConfig(AWSConfig),
			EMRClient:              emr.NewFromConfig(AWSConfig),
			GlueClient:             glue.NewFromConfig(AWSConfig),
			GrafanaClient:          grafana.NewFromConfig(AWSConfig),
			IAMClient:              iam.NewFromConfig(AWSConfig),
			KinesisClient:          kinesis.NewFromConfig(AWSConfig),
			LambdaClient:           lambda.NewFromConfig(AWSConfig),
			LightsailClient:        lightsail.NewFromConfig(AWSConfig),
			MQClient:               mq.NewFromConfig(AWSConfig),
			OpenSearchClient:       opensearch.NewFromConfig(AWSConfig),
			RDSClient:              rds.NewFromConfig(AWSConfig),
			RedshiftClient:         redshift.NewFromConfig(AWSConfig),
			Route53Client:          route53.NewFromConfig(AWSConfig),
			S3Client:               s3.NewFromConfig(AWSConfig),
			SecretsManagerClient:   secretsmanager.NewFromConfig(AWSConfig),
			SNSClient:              sns.NewFromConfig(AWSConfig),
			SQSClient:              sqs.NewFromConfig(AWSConfig),
			SSMClient:              ssm.NewFromConfig(AWSConfig),
			StepFunctionClient:     sfn.NewFromConfig(AWSConfig),

			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintInventoryPerRegion(AWSOutputDirectory, Verbosity)
	}
}

func runLambdasCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.LambdasModule{
			LambdaClient:        lambda.NewFromConfig(AWSConfig),
			IAMClient:           iam.NewFromConfig(AWSConfig),
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.PrintLambdas(AWSOutputDirectory, Verbosity)
	}
}

func runOutboundAssumedRolesCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.OutboundAssumedRolesModule{
			CloudTrailClient: cloudtrail.NewFromConfig(AWSConfig),

			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintOutboundRoleTrusts(OutboundAssumedRolesDays, AWSOutputDirectory, Verbosity)
	}
}

func runOrgsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.OrgModule{
			OrganizationsClient: organizations.NewFromConfig(AWSConfig),
			Caller:              *caller,
			AWSProfile:          profile,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
		}
		m.PrintOrgAccounts(AWSOutputDirectory, Verbosity)
	}
}

func runPermissionsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.IamPermissionsModule{
			IAMClient:     iam.NewFromConfig(AWSConfig),
			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSTableCols:  AWSTableCols,
			AWSOutputType: AWSOutputType,
		}
		m.PrintIamPermissions(AWSOutputDirectory, Verbosity, PermissionsPrincipal)
	}
}

func runPmapperCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.PmapperModule{
			Caller:              *caller,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.PrintPmapperData(AWSOutputDirectory, Verbosity)
	}
}

func runPrincipalsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.IamPrincipalsModule{
			IAMClient:           iam.NewFromConfig(AWSConfig),
			Caller:              *caller,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.PrintIamPrincipals(AWSOutputDirectory, Verbosity)
	}
}

func runRAMCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		ram := aws.RAMModule{
			RAMClient:     ram.NewFromConfig(AWSConfig),
			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		ram.PrintRAM(AWSOutputDirectory, Verbosity)

	}
}

func runResourceTrustsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		runResourceTrustsCommandWithProfile(cmd, args, profile)
	}
}

func runResourceTrustsCommandWithProfile(cmd *cobra.Command, args []string, profile string) {
	var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
	caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
	var KMSClient sdk.KMSClientInterface = kms.NewFromConfig(AWSConfig)
	var APIGatewayClient sdk.APIGatewayClientInterface = apigateway.NewFromConfig(AWSConfig)
	var EC2Client sdk.AWSEC2ClientInterface = ec2.NewFromConfig(AWSConfig)
	var OpenSearchClient sdk.OpenSearchClientInterface = opensearch.NewFromConfig(AWSConfig)

	if err != nil {
		return
	}
	m := aws.ResourceTrustsModule{
		KMSClient:        &KMSClient,
		APIGatewayClient: &APIGatewayClient,
		EC2Client:        &EC2Client,
		OpenSearchClient: &OpenSearchClient,

		Caller:             *caller,
		AWSProfileProvided: profile,
		Goroutines:         Goroutines,
		AWSRegions:         internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
		WrapTable:          AWSWrapTable,
		CloudFoxVersion:    cmd.Root().Version,
		AWSOutputType:      AWSOutputType,
		AWSTableCols:       AWSTableCols,
		AWSConfig:          AWSConfig,
	}
	m.PrintResources(AWSOutputDirectory, Verbosity, ResourceTrustsIncludeKms)
}

func runRoleTrustCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.RoleTrustsModule{
			IAMClient:           iam.NewFromConfig(AWSConfig),
			Caller:              *caller,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.PrintRoleTrusts(AWSOutputDirectory, Verbosity)
	}
}

func runRoute53Command(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.Route53Module{
			Route53Client: route53.NewFromConfig(AWSConfig),

			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintRoute53(AWSOutputDirectory, Verbosity)
	}
}

func runSecretsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.SecretsModule{
			SecretsManagerClient: secretsmanager.NewFromConfig(AWSConfig),
			SSMClient:            ssm.NewFromConfig(AWSConfig),

			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintSecrets(AWSOutputDirectory, Verbosity)
	}
}

func runTagsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.TagsModule{
			ResourceGroupsTaggingApiInterface: resourcegroupstaggingapi.NewFromConfig(AWSConfig),
			Caller:                            *caller,
			AWSRegions:                        internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:                        profile,
			Goroutines:                        Goroutines,
			WrapTable:                         AWSWrapTable,
			MaxResourcesPerRegion:             MaxResourcesPerRegion,
			AWSOutputType:                     AWSOutputType,
			AWSTableCols:                      AWSTableCols,
		}
		m.PrintTags(AWSOutputDirectory, Verbosity)
	}
}

func runWorkloadsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.WorkloadsModule{
			ECSClient:           ecs.NewFromConfig(AWSConfig),
			EC2Client:           ec2.NewFromConfig(AWSConfig),
			LambdaClient:        lambda.NewFromConfig(AWSConfig),
			AppRunnerClient:     apprunner.NewFromConfig(AWSConfig),
			IAMClient:           iam.NewFromConfig(AWSConfig),
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			SkipAdminCheck:      AWSSkipAdminCheck,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.PrintWorkloads(AWSOutputDirectory, Verbosity)
	}
}

func runDirectoryServicesCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.DirectoryModule{
			DSClient:      directoryservice.NewFromConfig(AWSConfig),
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.PrintDirectories(AWSOutputDirectory, Verbosity)
	}
}

func runECSTasksCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.ECSTasksModule{
			EC2Client: ec2.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			ECSClient: ecs.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			IAMClient: iam.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),

			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		m.ECSTasks(AWSOutputDirectory, Verbosity)
	}
}

func runENICommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.ElasticNetworkInterfacesModule{
			//EC2Client:                       ec2.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			EC2Client: ec2.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),

			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		m.ElasticNetworkInterfaces(AWSOutputDirectory, Verbosity)
	}
}

func runNetworkPortsCommand(cmd *cobra.Command, args []string) {
	for _, profile := range AWSProfiles {
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}
		m := aws.NetworkPortsModule{
			EC2Client:         ec2.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			ECSClient:         ecs.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			EFSClient:         efs.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			ElastiCacheClient: elasticache.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			ELBv2Client:       elasticloadbalancingv2.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			LightsailClient:   lightsail.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			RDSClient:         rds.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			Caller:            *caller,
			AWSRegions:        internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:        profile,
			Goroutines:        Goroutines,
			WrapTable:         AWSWrapTable,
			Verbosity:         Verbosity,
			AWSOutputType:     AWSOutputType,
			AWSTableCols:      AWSTableCols,
		}
		m.PrintNetworkPorts(AWSOutputDirectory)
	}
}

func runAllChecksCommand(cmd *cobra.Command, args []string) {
	Verbosity = 1
	for _, profile := range AWSProfiles {
		var AWSConfig = internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)
		caller, err := internal.AWSWhoami(profile, cmd.Root().Version, AWSMFAToken)
		if err != nil {
			continue
		}

		apiGatewayClient := apigateway.NewFromConfig(AWSConfig)
		apiGatewayv2Client := apigatewayv2.NewFromConfig(AWSConfig)
		appRunnerClient := apprunner.NewFromConfig(AWSConfig)
		athenaClient := athena.NewFromConfig(AWSConfig)
		cloud9Client := cloud9.NewFromConfig(AWSConfig)
		cloudFormationClient := cloudformation.NewFromConfig(AWSConfig)
		cloudfrontClient := cloudfront.NewFromConfig(AWSConfig)
		codeArtifactClient := codeartifact.NewFromConfig(AWSConfig)
		codeBuildClient := codebuild.NewFromConfig(AWSConfig)
		codeCommitClient := codecommit.NewFromConfig(AWSConfig)
		codeDeployClient := codedeploy.NewFromConfig(AWSConfig)
		dataPipelineClient := datapipeline.NewFromConfig(AWSConfig)
		dynamodbClient := dynamodb.NewFromConfig(AWSConfig)
		ec2Client := ec2.NewFromConfig(AWSConfig)
		ecrClient := ecr.NewFromConfig(AWSConfig)
		ecsClient := ecs.NewFromConfig(AWSConfig)
		efsClient := efs.NewFromConfig(AWSConfig)
		eksClient := eks.NewFromConfig(AWSConfig)
		elasticacheClient := elasticache.NewFromConfig(AWSConfig)
		elasticBeanstalkClient := elasticbeanstalk.NewFromConfig(AWSConfig)
		elbClient := elasticloadbalancing.NewFromConfig(AWSConfig)
		elbv2Client := elasticloadbalancingv2.NewFromConfig(AWSConfig)
		emrClient := emr.NewFromConfig(AWSConfig)
		fsxClient := fsx.NewFromConfig(AWSConfig)
		glueClient := glue.NewFromConfig(AWSConfig)
		grafanaClient := grafana.NewFromConfig(AWSConfig)
		iamClient := iam.NewFromConfig(AWSConfig)
		kinesisClient := kinesis.NewFromConfig(AWSConfig)
		lambdaClient := lambda.NewFromConfig(AWSConfig)
		lightsailClient := lightsail.NewFromConfig(AWSConfig)
		mqClient := mq.NewFromConfig(AWSConfig)
		openSearchClient := opensearch.NewFromConfig(AWSConfig)
		orgClient := organizations.NewFromConfig(AWSConfig)
		ramClient := ram.NewFromConfig(AWSConfig)
		rdsClient := rds.NewFromConfig(AWSConfig)
		redshiftClient := redshift.NewFromConfig(AWSConfig)
		resourceClient := resourcegroupstaggingapi.NewFromConfig(AWSConfig)
		route53Client := route53.NewFromConfig(AWSConfig)
		s3Client := s3.NewFromConfig(AWSConfig)
		sagemakerClient := sagemaker.NewFromConfig(AWSConfig)
		secretsManagerClient := secretsmanager.NewFromConfig(AWSConfig)
		snsClient := sns.NewFromConfig(AWSConfig)
		sqsClient := sqs.NewFromConfig(AWSConfig)
		ssmClient := ssm.NewFromConfig(AWSConfig)
		stepFunctionClient := sfn.NewFromConfig(AWSConfig)

		fmt.Printf("[%s] %s\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("Getting a lay of the land, aka \"What regions is this account using?\""))
		inventory2 := aws.Inventory2Module{
			APIGatewayClient:       apiGatewayClient,
			APIGatewayv2Client:     apiGatewayv2Client,
			AppRunnerClient:        appRunnerClient,
			AthenaClient:           athenaClient,
			Cloud9Client:           cloud9Client,
			CloudFormationClient:   cloudFormationClient,
			CloudfrontClient:       cloudfrontClient,
			CodeArtifactClient:     codeArtifactClient,
			CodeBuildClient:        codeBuildClient,
			CodeCommitClient:       codeCommitClient,
			CodeDeployClient:       codeDeployClient,
			DataPipelineClient:     dataPipelineClient,
			DynamoDBClient:         dynamodbClient,
			EC2Client:              ec2Client,
			ECSClient:              ecsClient,
			ECRClient:              ecrClient,
			EKSClient:              eksClient,
			ELBClient:              elbClient,
			ELBv2Client:            elbv2Client,
			ElasticacheClient:      elasticacheClient,
			ElasticBeanstalkClient: elasticBeanstalkClient,
			EMRClient:              emrClient,
			GlueClient:             glueClient,
			GrafanaClient:          grafanaClient,
			IAMClient:              iamClient,
			KinesisClient:          kinesisClient,
			LambdaClient:           lambdaClient,
			LightsailClient:        lightsailClient,
			MQClient:               mqClient,
			OpenSearchClient:       openSearchClient,
			RDSClient:              rdsClient,
			RedshiftClient:         redshiftClient,
			Route53Client:          route53Client,
			S3Client:               s3Client,
			SecretsManagerClient:   secretsManagerClient,
			SNSClient:              snsClient,
			SQSClient:              sqsClient,
			SSMClient:              ssmClient,
			StepFunctionClient:     stepFunctionClient,
			Caller:                 *caller,
			AWSRegions:             internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:             profile,
			Goroutines:             Goroutines,
			WrapTable:              AWSWrapTable,
			AWSOutputType:          AWSOutputType,
			AWSTableCols:           AWSTableCols,
		}
		inventory2.PrintInventoryPerRegion(AWSOutputDirectory, Verbosity)

		tagsMod := aws.TagsModule{
			ResourceGroupsTaggingApiInterface: resourceClient,
			Caller:                            *caller,
			AWSRegions:                        internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:                        profile,
			Goroutines:                        Goroutines,
			MaxResourcesPerRegion:             1000,
		}
		var verbosityOverride int = 1
		tagsMod.PrintTags(AWSOutputDirectory, verbosityOverride)

		orgMod := aws.OrgModule{
			OrganizationsClient: orgClient,
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
		}
		orgMod.PrintOrgAccounts(AWSOutputDirectory, Verbosity)

		// Service and endpoint enum section
		fmt.Printf("[%s] %s\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("Gathering the info you'll want for your application & service enumeration needs."))
		instances := aws.InstancesModule{
			EC2Client:              ec2Client,
			IAMClient:              iamClient,
			Caller:                 *caller,
			AWSRegions:             internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			SkipAdminCheck:         AWSSkipAdminCheck,
			UserDataAttributesOnly: false,
			AWSProfile:             profile,
			WrapTable:              AWSWrapTable,
			AWSOutputType:          AWSOutputType,
			AWSTableCols:           AWSTableCols,
			PmapperDataBasePath:    PmapperDataBasePath,
		}
		instances.Instances(InstancesFilter, AWSOutputDirectory, Verbosity)
		route53 := aws.Route53Module{
			Route53Client: route53Client,
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
		}

		lambdasMod := aws.LambdasModule{
			LambdaClient:        lambdaClient,
			IAMClient:           iamClient,
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		lambdasMod.PrintLambdas(AWSOutputDirectory, Verbosity)

		route53.PrintRoute53(AWSOutputDirectory, Verbosity)

		filesystems := aws.FilesystemsModule{
			EFSClient:     efsClient,
			FSxClient:     fsxClient,
			Caller:        *caller,
			AWSProfile:    profile,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		filesystems.PrintFilesystems(AWSOutputDirectory, Verbosity)

		endpoints := aws.EndpointsModule{
			EKSClient:          eksClient,
			S3Client:           s3Client,
			LambdaClient:       lambdaClient,
			RDSClient:          rdsClient,
			APIGatewayv2Client: apiGatewayv2Client,
			APIGatewayClient:   apiGatewayClient,
			ELBClient:          elbClient,
			ELBv2Client:        elbv2Client,
			MQClient:           mqClient,
			OpenSearchClient:   openSearchClient,
			GrafanaClient:      grafanaClient,
			RedshiftClient:     redshiftClient,
			CloudfrontClient:   cloudfrontClient,
			AppRunnerClient:    appRunnerClient,
			LightsailClient:    lightsailClient,
			Caller:             *caller,
			AWSRegions:         internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:         profile,
			Goroutines:         Goroutines,
			WrapTable:          AWSWrapTable,
			AWSOutputType:      AWSOutputType,
			AWSTableCols:       AWSTableCols,
		}

		endpoints.PrintEndpoints(AWSOutputDirectory, Verbosity)

		gateways := aws.ApiGwModule{
			APIGatewayv2Client: apiGatewayv2Client,
			APIGatewayClient:   apiGatewayClient,
			Caller:             *caller,
			AWSRegions:         internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:         profile,
			Goroutines:         Goroutines,
			WrapTable:          AWSWrapTable,
		}

		gateways.PrintApiGws(AWSOutputDirectory, Verbosity)

		databases := aws.DatabasesModule{
			RDSClient:      rdsClient,
			RedshiftClient: redshiftClient,
			DynamoDBClient: dynamodbClient,
			Caller:         *caller,
			AWSProfile:     profile,
			AWSRegions:     internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			Goroutines:     Goroutines,
			WrapTable:      AWSWrapTable,
			AWSOutputType:  AWSOutputType,
			AWSTableCols:   AWSTableCols,
		}

		databases.PrintDatabases(AWSOutputDirectory, Verbosity)

		ecstasks := aws.ECSTasksModule{
			EC2Client:           ec2Client,
			ECSClient:           ecsClient,
			IAMClient:           iamClient,
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		ecstasks.ECSTasks(AWSOutputDirectory, Verbosity)

		eksCommand := aws.EKSModule{
			EKSClient:           eksClient,
			IAMClient:           iamClient,
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			SkipAdminCheck:      AWSSkipAdminCheck,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		eksCommand.EKS(AWSOutputDirectory, Verbosity)

		elasticnetworkinterfaces := aws.ElasticNetworkInterfacesModule{
			EC2Client:     ec2Client,
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		elasticnetworkinterfaces.ElasticNetworkInterfaces(AWSOutputDirectory, Verbosity)

		// Secrets section
		fmt.Printf("[%s] %s\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("Looking for secrets hidden between the seat cushions."))

		ec2UserData := aws.InstancesModule{
			EC2Client:              ec2Client,
			IAMClient:              iamClient,
			Caller:                 *caller,
			AWSRegions:             internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			UserDataAttributesOnly: true,
			AWSProfile:             profile,
			Goroutines:             Goroutines,
			WrapTable:              AWSWrapTable,
			AWSOutputType:          AWSOutputType,
			AWSTableCols:           AWSTableCols,
		}
		ec2UserData.Instances(InstancesFilter, AWSOutputDirectory, Verbosity)
		envsMod := aws.EnvsModule{
			Caller:          *caller,
			AWSRegions:      internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:      profile,
			ECSClient:       ecsClient,
			AppRunnerClient: appRunnerClient,
			LambdaClient:    lambdaClient,
			LightsailClient: lightsailClient,
			SagemakerClient: sagemakerClient,
			Goroutines:      Goroutines,
			WrapTable:       AWSWrapTable,
			AWSOutputType:   AWSOutputType,
			AWSTableCols:    AWSTableCols,
		}
		envsMod.PrintEnvs(AWSOutputDirectory, Verbosity)

		cfMod := aws.CloudformationModule{
			CloudFormationClient: cloudFormationClient,
			Caller:               *caller,
			AWSRegions:           internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:           profile,
			Goroutines:           Goroutines,
			WrapTable:            AWSWrapTable,
			AWSOutputType:        AWSOutputType,
			AWSTableCols:         AWSTableCols,
		}
		cfMod.PrintCloudformationStacks(AWSOutputDirectory, Verbosity)

		// CPT Enum
		//fmt.Printf("[%s] %s\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("Gathering some other info that is often useful."))
		fmt.Printf("[%s] %s\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("Arming you with the data you'll need for privesc quests."))

		// cloudFoxS3Client := aws.CloudFoxS3Client{
		// 	S3Client:   s3.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
		// 	Caller:     *caller,
		// 	AWSRegions: internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
		// 	AWSProfile: profile,
		// }

		buckets := aws.BucketsModule{
			S3Client:      s3.NewFromConfig(internal.AWSConfigFileLoader(profile, cmd.Root().Version, AWSMFAToken)),
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		buckets.PrintBuckets(AWSOutputDirectory, Verbosity)

		ecr := aws.ECRModule{
			ECRClient:     ecrClient,
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		ecr.PrintECR(AWSOutputDirectory, Verbosity)

		secrets := aws.SecretsModule{
			SecretsManagerClient: secretsManagerClient,
			SSMClient:            ssmClient,
			Caller:               *caller,
			AWSRegions:           internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:           profile,
			Goroutines:           Goroutines,
			WrapTable:            AWSWrapTable,
			AWSOutputType:        AWSOutputType,
			AWSTableCols:         AWSTableCols,
		}
		secrets.PrintSecrets(AWSOutputDirectory, Verbosity)

		ram := aws.RAMModule{
			RAMClient:     ramClient,
			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		ram.PrintRAM(AWSOutputDirectory, Verbosity)

		networkPorts := aws.NetworkPortsModule{
			EC2Client:         ec2Client,
			ECSClient:         ecsClient,
			EFSClient:         efsClient,
			ElastiCacheClient: elasticacheClient,
			ELBv2Client:       elbv2Client,
			LightsailClient:   lightsailClient,
			RDSClient:         rdsClient,
			Caller:            *caller,
			AWSProfile:        profile,
			Goroutines:        Goroutines,
			AWSRegions:        internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSOutputType:     AWSOutputType,
			AWSTableCols:      AWSTableCols,
		}
		networkPorts.PrintNetworkPorts(AWSOutputDirectory)

		sqsMod := aws.SQSModule{
			SQSClient:     sqsClient,
			StorePolicies: StoreSQSAccessPolicies,
			Caller:        *caller,
			AWSRegions:    internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		sqsMod.PrintSQS(AWSOutputDirectory, Verbosity)

		cloudFoxSNSClient := aws.InitCloudFoxSNSClient(*caller, profile, cmd.Root().Version, Goroutines, AWSWrapTable, AWSMFAToken)
		cloudFoxSNSClient.PrintSNS(AWSOutputDirectory, Verbosity)

		runResourceTrustsCommandWithProfile(cmd, args, profile)

		codeBuildCommand := aws.CodeBuildModule{
			CodeBuildClient:     codeBuildClient,
			Caller:              *caller,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		codeBuildCommand.PrintCodeBuildProjects(AWSOutputDirectory, Verbosity)

		// IAM privesc section
		fmt.Printf("[%s] %s\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("IAM is complicated. Complicated usually means misconfigurations. You'll want to pay attention here."))
		principals := aws.IamPrincipalsModule{
			IAMClient:     iamClient,
			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}

		principals.PrintIamPrincipals(AWSOutputDirectory, Verbosity)
		permissions := aws.IamPermissionsModule{
			IAMClient:  iamClient,
			Caller:     *caller,
			AWSProfile: profile,
			Goroutines: Goroutines,
			WrapTable:  AWSWrapTable,
		}
		permissions.PrintIamPermissions(AWSOutputDirectory, Verbosity, PermissionsPrincipal)
		accessKeys := aws.AccessKeysModule{
			IAMClient:     iam.NewFromConfig(AWSConfig),
			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		accessKeys.PrintAccessKeys(AccessKeysFilter, AWSOutputDirectory, Verbosity)
		roleTrusts := aws.RoleTrustsModule{
			IAMClient:      iamClient,
			Caller:         *caller,
			AWSProfile:     profile,
			Goroutines:     Goroutines,
			SkipAdminCheck: AWSSkipAdminCheck,
			WrapTable:      AWSWrapTable,
			AWSOutputType:  AWSOutputType,
			AWSTableCols:   AWSTableCols,
		}
		roleTrusts.PrintRoleTrusts(AWSOutputDirectory, Verbosity)

		pmapperCommand := aws.PmapperModule{
			Caller:        *caller,
			AWSProfile:    profile,
			Goroutines:    Goroutines,
			WrapTable:     AWSWrapTable,
			AWSOutputType: AWSOutputType,
			AWSTableCols:  AWSTableCols,
		}
		pmapperCommand.PrintPmapperData(AWSOutputDirectory, Verbosity)

		iamSimulator := aws.IamSimulatorModule{
			IAMClient:          iamClient,
			Caller:             *caller,
			AWSProfileProvided: profile,
			Goroutines:         Goroutines,
			WrapTable:          AWSWrapTable,
			AWSOutputType:      AWSOutputType,
			AWSTableCols:       AWSTableCols,
		}
		iamSimulator.PrintIamSimulator(SimulatorPrincipal, SimulatorAction, SimulatorResource, AWSOutputDirectory, Verbosity)

		workloads := aws.WorkloadsModule{
			ECSClient:           ecsClient,
			EC2Client:           ec2Client,
			LambdaClient:        lambdaClient,
			AppRunnerClient:     appRunnerClient,
			IAMClient:           iamClient,
			Caller:              *caller,
			AWSRegions:          internal.GetEnabledRegions(profile, cmd.Root().Version, AWSMFAToken),
			SkipAdminCheck:      AWSSkipAdminCheck,
			AWSProfile:          profile,
			Goroutines:          Goroutines,
			WrapTable:           AWSWrapTable,
			AWSOutputType:       AWSOutputType,
			AWSTableCols:        AWSTableCols,
			PmapperDataBasePath: PmapperDataBasePath,
		}
		workloads.PrintWorkloads(AWSOutputDirectory, Verbosity)

		fmt.Printf("[%s] %s\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("That's it! Check your output files for situational awareness and check your loot files for next steps."))
		fmt.Printf("[%s] %s\n\n", cyan(emoji.Sprintf(":fox:cloudfox :fox:")), green("FYI, we skipped the outbound-assumed-roles module in all-checks (really long run time). Make sure to try it out manually."))
	}
}

var CapeTuiCmd = &cobra.Command{
	Use:     "tui",
	Aliases: []string{"TUI", "view", "report"},
	Short:   "View Cape's output in a TUI",
	Long: "\nUse case examples:\n" +
		os.Args[0] + " aws cape tui -l /path/to/profiles-used-for-cape.txt",
	//PreRun:  awsPreRun,
	Run: runCapeTUICommand,
	//PostRun: awsPostRun,
}

func init() {
	cobra.OnInitialize(initAWSProfiles)

	// Role Trusts Module Flags
	RoleTrustCommand.Flags().StringVarP(&RoleTrustFilter, "filter", "f", "all", "[AccountNumber | PrincipalARN | PrincipalName | ServiceName]")

	// Map Access Keys Module Flags
	AccessKeysCommand.Flags().StringVarP(&AccessKeysFilter, "filter", "f", "none", "Access key ID to search for")

	// Instances Map Module Flags
	InstancesCommand.Flags().StringVarP(&InstancesFilter, "filter", "f", "all", "[InstanceID | InstanceIDsFile]")
	InstancesCommand.Flags().BoolVarP(&InstanceMapUserDataAttributesOnly, "userdata", "u", false, "Use this flag to retrieve only the userData attribute from EC2 instances.")

	// SQS module flags
	SQSCommand.Flags().BoolVarP(&StoreSQSAccessPolicies, "policies", "", false, "Store all flagged access policies along with the output")

	// SNS module flags
	SNSCommand.Flags().BoolVarP(&StoreSNSAccessPolicies, "policies", "", false, "Store all flagged access policies along with the output")

	//  outbound-assumed-roles module flags
	OutboundAssumedRolesCommand.Flags().IntVarP(&OutboundAssumedRolesDays, "days", "d", -7, "How many days of CloudTrail events should we go back and look at.")

	//  iam-simulator module flags
	IamSimulatorCommand.Flags().StringVar(&SimulatorPrincipal, "principal", "", "Principal Arn")
	IamSimulatorCommand.Flags().StringVar(&SimulatorAction, "action", "", "Action")
	IamSimulatorCommand.Flags().StringVar(&SimulatorResource, "resource", "*", "Resource")
	IamSimulatorCommand.Flags().BoolVar(&IamSimulatorAdminCheckOnly, "admin-check-only", false, "Only check check if principals are admin")

	//  iam-simulator module flags
	PermissionsCommand.Flags().StringVar(&PermissionsPrincipal, "principal", "", "Principal Arn")

	// tags module flags
	TagsCommand.Flags().IntVarP(&MaxResourcesPerRegion, "max-resources-per-region", "m", 0, "Maximum number of resources to enumerate per region. Set to 0 to enumerate all resources.")

	// buckets command flags (for bucket policies)
	BucketsCommand.Flags().BoolVarP(&CheckBucketPolicies, "with-policies", "", false, "Analyze bucket policies (this is already done in the resource-trusts command)")

	// cape command flags
	CapeCommand.Flags().BoolVar(&CapeAdminOnly, "admin-only", false, "Only return paths that lead to an admin role - much faster")
	//CapeCommand.Flags().StringVar(&CapeJobName, "job-name", "", "Name of the cape job")
	// flag that accepts a list of arns to ignore
	CapeCommand.Flags().StringVar(&CapeArnIgnoreList, "arn-ignore-list", "", "File containing a list of ARNs to ignore separated by newlines")

	// cape tui command flags
	CapeTuiCmd.Flags().BoolVar(&CapeAdminOnly, "admin-only", false, "Only return paths that lead to an admin role - much faster")

	// Resource Trust command flags
	ResourceTrustsCommand.Flags().BoolVar(&ResourceTrustsIncludeKms, "include-kms", false, "Include KMS keys in the output")

	// Global flags for the AWS modules
	AWSCommands.PersistentFlags().StringVarP(&AWSProfile, "profile", "p", "", "AWS CLI Profile Name")
	AWSCommands.PersistentFlags().StringVarP(&AWSProfilesList, "profiles-list", "l", "", "File containing a AWS CLI profile names separated by newlines")
	AWSCommands.PersistentFlags().BoolVarP(&AWSAllProfiles, "all-profiles", "a", false, "Use all AWS CLI profiles in AWS credentials file")
	AWSCommands.PersistentFlags().BoolVarP(&AWSConfirm, "yes", "y", false, "Non-interactive mode (like apt/yum)")
	AWSCommands.PersistentFlags().StringVarP(&AWSOutputType, "output", "o", "brief", "[\"brief\" | \"wide\" ]")
	AWSCommands.PersistentFlags().IntVarP(&Verbosity, "verbosity", "v", 2, "1 = Print control messages only\n2 = Print control messages, module output\n3 = Print control messages, module output, and loot file output\n")
	AWSCommands.PersistentFlags().StringVar(&AWSOutputDirectory, "outdir", defaultOutputDir, "Output Directory ")
	AWSCommands.PersistentFlags().IntVarP(&Goroutines, "max-goroutines", "g", 30, "Maximum number of concurrent goroutines")
	AWSCommands.PersistentFlags().BoolVar(&AWSSkipAdminCheck, "skip-admin-check", false, "Skip check to determine if role is an Admin")
	AWSCommands.PersistentFlags().BoolVarP(&AWSWrapTable, "wrap", "w", false, "Wrap table to fit in terminal (complicates grepping)")
	AWSCommands.PersistentFlags().BoolVarP(&AWSUseCache, "cached", "c", false, "Load cached data from disk. Faster, but if changes have been recently made you'll miss them")
	AWSCommands.PersistentFlags().StringVarP(&AWSTableCols, "cols", "t", "", "Comma separated list of columns to display in table output")
	AWSCommands.PersistentFlags().StringVar(&AWSMFAToken, "mfa-token", "", "MFA Token")
	AWSCommands.PersistentFlags().StringVar(&PmapperDataBasePath, "pmapper-data-basepath", "", "Supply the base path for the pmapper data files (useful if you have copied them from another machine)\nPoint to the parent directory that contains all of the pmapper data by account numbers. \n\tExample: /path/to/com.nccgroup.principalmapper/\n\tExample: ./pmapperdata/")

	AWSCommands.AddCommand(
		AccessKeysCommand,
		AllChecksCommand,
		ApiGwCommand,
		BucketsCommand,
		CapeCommand,
		CloudformationCommand,
		CodeBuildCommand,
		DatabasesCommand,
		ECSTasksCommand,
		ECRCommand,
		EKSCommand,
		ElasticNetworkInterfacesCommand,
		EndpointsCommand,
		EnvsCommand,
		FilesystemsCommand,
		//GraphCommand,
		IamSimulatorCommand,
		InstancesCommand,
		InventoryCommand,
		LambdasCommand,
		NetworkPortsCommand,
		OrgsCommand,
		OutboundAssumedRolesCommand,
		PermissionsCommand,
		PrincipalsCommand,
		PmapperCommand,
		RAMCommand,
		ResourceTrustsCommand,
		RoleTrustCommand,
		Route53Command,
		SQSCommand,
		SNSCommand,
		SecretsCommand,
		TagsCommand,
		WorkloadsCommand,
		DirectoryServicesCommand,
	)

	CapeCommand.AddCommand(
		CapeTuiCmd,
	)

}
