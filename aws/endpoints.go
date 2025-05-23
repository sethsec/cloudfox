package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/BishopFox/cloudfox/aws/sdk"
	"github.com/BishopFox/cloudfox/internal"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigatewayTypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"

	apigatewayV2Types "github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	"github.com/aws/aws-sdk-go-v2/service/apprunner"
	apprunnerTypes "github.com/aws/aws-sdk-go-v2/service/apprunner/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/grafana"
	"github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/aws/aws-sdk-go-v2/service/mq"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
	"github.com/bishopfox/awsservicemap"
	"github.com/sirupsen/logrus"
)

type EndpointsModule struct {
	// General configuration data
	LambdaClient       sdk.LambdaClientInterface
	EKSClient          sdk.EKSClientInterface
	MQClient           *mq.Client
	OpenSearchClient   *opensearch.Client
	GrafanaClient      *grafana.Client
	ELBv2Client        *elasticloadbalancingv2.Client
	ELBClient          *elasticloadbalancing.Client
	APIGatewayClient   *apigateway.Client
	APIGatewayv2Client *apigatewayv2.Client
	RDSClient          *rds.Client
	RedshiftClient     *redshift.Client
	S3Client           *s3.Client
	CloudfrontClient   *cloudfront.Client
	AppRunnerClient    *apprunner.Client
	LightsailClient    *lightsail.Client

	Caller        sts.GetCallerIdentityOutput
	AWSRegions    []string
	AWSOutputType string
	AWSTableCols  string

	Goroutines int
	AWSProfile string
	WrapTable  bool

	// Main module data
	Endpoints      []Endpoint
	CommandCounter internal.CommandCounter
	Errors         []string
	// Used to store output data for pretty printing
	output internal.OutputData2
	modLog *logrus.Entry
}

type Endpoint struct {
	AWSService string
	Region     string
	Name       string
	Endpoint   string
	Port       int32
	Protocol   string
	Public     string
}

var oe *smithy.OperationError

func (m *EndpointsModule) PrintEndpoints(outputDirectory string, verbosity int) {
	// These struct values are used by the output module
	m.output.Verbosity = verbosity
	m.output.Directory = outputDirectory
	m.output.CallingModule = "endpoints"
	m.modLog = internal.TxtLog.WithFields(logrus.Fields{
		"module": m.output.CallingModule,
	})
	if m.AWSProfile == "" {
		m.AWSProfile = internal.BuildAWSPath(m.Caller)
	}

	fmt.Printf("[%s][%s] Enumerating endpoints for account %s.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), aws.ToString(m.Caller.Account))
	fmt.Printf("[%s][%s] Supported Services: App Runner, APIGateway, ApiGatewayV2, Cloudfront, EKS, ELB, ELBv2, Grafana, \n", cyan(m.output.CallingModule), cyan(m.AWSProfile))
	fmt.Printf("[%s][%s] \t\t\tLambda, MQ, OpenSearch, Redshift, RDS\n", cyan(m.output.CallingModule), cyan(m.AWSProfile))

	wg := new(sync.WaitGroup)
	semaphore := make(chan struct{}, m.Goroutines)
	// Create a channel to signal the spinner aka task status goroutine to finish
	spinnerDone := make(chan bool)
	//fire up the task status spinner/updated
	go internal.SpinUntil(m.output.CallingModule, &m.CommandCounter, spinnerDone, "tasks")

	//create a channel to receive the objects
	dataReceiver := make(chan Endpoint)

	// Create a channel to signal to stop
	receiverDone := make(chan bool)

	go m.Receiver(dataReceiver, receiverDone)

	//execute global checks -- removing from now. not sure i want s3 data in here
	// wg.Add(1)
	// go m.getS3EndpointsPerRegion(wg)
	wg.Add(1)
	go m.getCloudfrontEndpoints(wg, semaphore, dataReceiver)

	//execute regional checks

	for _, region := range m.AWSRegions {
		wg.Add(1)
		go m.executeChecks(region, wg, semaphore, dataReceiver)
	}

	// for _, r := range utils.GetRegionsForService(m.AWSProfile, "apprunner") {
	// 	fmt.Println()
	// 	fmt.Println(r)
	// 	m.getAppRunnerEndpointsPerRegion(r, wg)
	// }

	wg.Wait()
	//time.Sleep(time.Second * 2)

	// Send a message to the spinner goroutine to close the channel and stop
	spinnerDone <- true
	<-spinnerDone
	receiverDone <- true
	<-receiverDone

	sort.Slice(m.Endpoints, func(i, j int) bool {
		return m.Endpoints[i].AWSService < m.Endpoints[j].AWSService
	})

	m.output.Headers = []string{
		"Account",
		"Service",
		"Region",
		"Name",
		"Endpoint",
		"Port",
		"Protocol",
		"Public",
	}

	// If the user specified table columns, use those.
	// If the user specified -o wide, use the wide default cols for this module.
	// Otherwise, use the hardcoded default cols for this module.
	var tableCols []string
	// If the user specified table columns, use those.
	if m.AWSTableCols != "" {
		// If the user specified wide as the output format, use these columns.
		// remove any spaces between any commas and the first letter after the commas
		m.AWSTableCols = strings.ReplaceAll(m.AWSTableCols, ", ", ",")
		m.AWSTableCols = strings.ReplaceAll(m.AWSTableCols, ",  ", ",")
		tableCols = strings.Split(m.AWSTableCols, ",")
		// If the user specified wide as the output format, use these columns.
	} else if m.AWSOutputType == "wide" {
		tableCols = []string{
			"Account",
			"Service",
			"Region",
			"Name",
			"Endpoint",
			"Port",
			"Protocol",
			"Public",
		}
		// Otherwise, use the default columns.
	} else {
		tableCols = []string{
			"Service",
			"Region",
			"Name",
			"Endpoint",
			"Port",
			"Protocol",
			"Public",
		}
	}

	// Table rows
	for i := range m.Endpoints {
		m.output.Body = append(
			m.output.Body,
			[]string{
				aws.ToString(m.Caller.Account),
				m.Endpoints[i].AWSService,
				m.Endpoints[i].Region,
				m.Endpoints[i].Name,
				m.Endpoints[i].Endpoint,
				strconv.Itoa(int(m.Endpoints[i].Port)),
				m.Endpoints[i].Protocol,
				m.Endpoints[i].Public,
			},
		)

	}
	if len(m.output.Body) > 0 {
		m.output.FilePath = filepath.Join(outputDirectory, "cloudfox-output", "aws", fmt.Sprintf("%s-%s", m.AWSProfile, aws.ToString(m.Caller.Account)))
		o := internal.OutputClient{
			Verbosity:     verbosity,
			CallingModule: m.output.CallingModule,
			Table: internal.TableClient{
				Wrap: m.WrapTable,
			},
		}
		o.Table.TableFiles = append(o.Table.TableFiles, internal.TableFile{
			Header:    m.output.Headers,
			Body:      m.output.Body,
			TableCols: tableCols,
			Name:      m.output.CallingModule,
		})
		o.PrefixIdentifier = m.AWSProfile
		o.Table.DirectoryName = filepath.Join(outputDirectory, "cloudfox-output", "aws", fmt.Sprintf("%s-%s", m.AWSProfile, aws.ToString(m.Caller.Account)))
		o.WriteFullOutput(o.Table.TableFiles, nil)
		m.writeLoot(o.Table.DirectoryName, verbosity)
		fmt.Printf("[%s][%s] %s endpoints found.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), strconv.Itoa(len(m.output.Body)))
	} else {
		fmt.Printf("[%s][%s] No endpoints found, skipping the creation of an output file.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile))
	}
	fmt.Printf("[%s][%s] For context and next steps: https://github.com/BishopFox/cloudfox/wiki/AWS-Commands#%s\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), m.output.CallingModule)
	// This works great to print errors out after the module but i'm not really sure i want that.
	// sort.Slice(m.Errors, func(i, j int) bool {
	// 	return m.Errors[i] < m.Errors[j]
	// })
	// for _, e := range m.Errors {
	// 	fmt.Printf("[%s][%s] %s\n", cyan(m.output.CallingModule),  cyan(m.AWSProfile), e)
	// }

}

func (m *EndpointsModule) Receiver(receiver chan Endpoint, receiverDone chan bool) {
	defer close(receiverDone)
	for {
		select {
		case data := <-receiver:
			m.Endpoints = append(m.Endpoints, data)
		case <-receiverDone:
			receiverDone <- true
			return
		}
	}
}

func (m *EndpointsModule) executeChecks(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer wg.Done()
	// check the concurrency semaphore
	// semaphore <- struct{}{}
	// defer func() {
	// 	<-semaphore
	// }()

	servicemap := &awsservicemap.AwsServiceMap{
		JsonFileSource: "DOWNLOAD_FROM_AWS",
	}
	res, err := servicemap.IsServiceInRegion("lambda", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		go m.getLambdaFunctionsPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("eks", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		go m.getEksClustersPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("mq", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		go m.getMqBrokersPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("es", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		m.getOpenSearchPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("grafana", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		m.getGrafanaEndPointsPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("elb", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		go m.getELBv2ListenersPerRegion(r, wg, semaphore, dataReceiver)

		m.CommandCounter.Total++
		wg.Add(1)
		go m.getELBListenersPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("apigateway", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		go m.getAPIGatewayAPIsPerRegion(r, wg, semaphore, dataReceiver)

		m.CommandCounter.Total++
		wg.Add(1)
		go m.getAPIGatewayVIPsPerRegion(r, wg, semaphore, dataReceiver)

		m.CommandCounter.Total++
		wg.Add(1)
		go m.getAPIGatewayv2APIsPerRegion(r, wg, semaphore, dataReceiver)

		m.CommandCounter.Total++
		wg.Add(1)
		go m.getAPIGatewayv2VIPsPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("rds", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		go m.getRdsClustersPerRegion(r, wg, semaphore, dataReceiver)
	}
	res, err = servicemap.IsServiceInRegion("redshift", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		m.getRedshiftEndPointsPerRegion(r, wg, semaphore, dataReceiver)
	}

	//apprunner is not supported by the aws json so we have to call it in every region
	m.CommandCounter.Total++
	wg.Add(1)
	go m.getAppRunnerEndpointsPerRegion(r, wg, semaphore, dataReceiver)

	res, err = servicemap.IsServiceInRegion("lightsail", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {
		m.CommandCounter.Total++
		wg.Add(1)
		go m.getLightsailContainerEndpointsPerRegion(r, wg, semaphore, dataReceiver)
	}
}

func (m *EndpointsModule) writeLoot(outputDirectory string, verbosity int) {
	path := filepath.Join(outputDirectory, "loot")
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		panic(err.Error())
	}
	f := filepath.Join(path, "endpoints-UrlsOnly.txt")

	var out string

	for _, endpoint := range m.Endpoints {
		out = out + fmt.Sprintln(endpoint.Endpoint)
	}

	err = os.WriteFile(f, []byte(out), 0644)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		panic(err.Error())
	}

	if verbosity > 2 {
		fmt.Println()
		fmt.Printf("[%s][%s] %s \n", cyan(m.output.CallingModule), cyan(m.AWSProfile), green("Feed this endpoints into nmap and something like gowitness/aquatone for screenshots."))
		fmt.Print(out)
		fmt.Printf("[%s][%s] %s \n\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), green("End of loot file."))
	}

	fmt.Printf("[%s][%s] Loot written to [%s]\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), f)

}

func (m *EndpointsModule) getLambdaFunctionsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	// "PaginationMarker" is a control variable used for output continuity, as AWS return the output in pages.
	var public string

	Functions, err := sdk.CachedLambdaListFunctions(m.LambdaClient, aws.ToString(m.Caller.Account), r)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	for _, function := range Functions {
		name := aws.ToString(function.FunctionName)
		FunctionDetails, err := sdk.CachedLambdaGetFunctionUrlConfig(m.LambdaClient, aws.ToString(m.Caller.Account), r, name)
		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			continue
		}
		endpoint := aws.ToString(FunctionDetails.FunctionUrl)

		if FunctionDetails.AuthType == "NONE" {
			public = "True"
		} else {
			public = "False"
		}

		dataReceiver <- Endpoint{
			AWSService: "Lambda",
			Region:     r,
			Name:       name,
			Endpoint:   endpoint,
			Port:       443,
			Protocol:   "https",
			Public:     public,
		}
		//fmt.Println(endpoint, name, roleArn)
	}

}

func (m *EndpointsModule) getEksClustersPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	// "PaginationMarker" is a control variable used for output continuity, as AWS return the output in pages.

	Clusters, err := sdk.CachedEKSListClusters(m.EKSClient, aws.ToString(m.Caller.Account), r)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	for _, cluster := range Clusters {
		ClusterDetails, err := sdk.CachedEKSDescribeCluster(m.EKSClient, aws.ToString(m.Caller.Account), r, cluster)

		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			continue
		}
		var endpoint string
		var name string
		var public string
		vpcConfig := ClusterDetails.ResourcesVpcConfig.EndpointPublicAccess
		if vpcConfig {
			//
			if ClusterDetails.ResourcesVpcConfig.PublicAccessCidrs[0] == "0.0.0.0/0" {
				public = "True"
			} else {
				public = "False"
			}
		}

		endpoint = aws.ToString(ClusterDetails.Endpoint)
		name = aws.ToString(ClusterDetails.Name)
		dataReceiver <- Endpoint{
			AWSService: "Eks",
			Region:     r,
			Name:       name,
			Endpoint:   endpoint,
			Port:       443,
			Protocol:   "https",
			Public:     public,
		}

	}

}

func (m *EndpointsModule) getMqBrokersPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++

	BrokerSummaries, err := sdk.CachedMQListBrokers(m.MQClient, aws.ToString(m.Caller.Account), r)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	var public string
	for _, broker := range BrokerSummaries {
		name := aws.ToString(broker.BrokerName)
		id := broker.BrokerId

		BrokerDetails, err := m.MQClient.DescribeBroker(
			context.TODO(),
			&(mq.DescribeBrokerInput{
				BrokerId: id,
			}),
			func(o *mq.Options) {
				o.Region = r
			},
		)
		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			continue
		}
		if aws.ToBool(BrokerDetails.PubliclyAccessible) {
			public = "True"
		} else {
			public = "False"
		}

		endpoint := aws.ToString(BrokerDetails.BrokerInstances[0].ConsoleURL)

		dataReceiver <- Endpoint{
			AWSService: "Amazon MQ",
			Region:     r,
			Name:       name,
			Endpoint:   endpoint,
			Port:       443,
			Protocol:   "https",
			Public:     public,
		}

	}

}

func (m *EndpointsModule) getOpenSearchPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++

	DomainNames, err := sdk.CachedOpenSearchListDomainNames(m.OpenSearchClient, aws.ToString(m.Caller.Account), r)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	for _, domainName := range DomainNames {
		name := aws.ToString(domainName.DomainName)

		DomainStatus, err := sdk.CachedOpenSearchDescribeDomain(m.OpenSearchClient, aws.ToString(m.Caller.Account), r, name)

		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			return
		}

		rawEndpoint := DomainStatus.Endpoint
		var endpoint string
		var kibanaEndpoint string

		// This exits the function if an opensearch domain exists but there is no endpoint
		if rawEndpoint == nil {
			return
		} else {
			endpoint = fmt.Sprintf("https://%s", aws.ToString(rawEndpoint))
			kibanaEndpoint = fmt.Sprintf("https://%s/_plugin/kibana/", aws.ToString(rawEndpoint))
		}

		//fmt.Println(endpoint)

		public := "Unknown"
		domainConfig, err := sdk.CachedOpenSearchDescribeDomainConfig(m.OpenSearchClient, aws.ToString(m.Caller.Account), r, name)
		if err != nil {
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			return
		}
		if aws.ToBool(domainConfig.AdvancedSecurityOptions.Options.Enabled) {
			public = "False"
		} else {
			public = "True"
		}

		dataReceiver <- Endpoint{
			AWSService: "OpenSearch",
			Region:     r,
			Name:       name,
			Endpoint:   endpoint,
			Port:       443,
			Protocol:   "https",
			Public:     public,
		}
		dataReceiver <- Endpoint{
			AWSService: "OpenSearch",
			Region:     r,
			Name:       name,
			Endpoint:   kibanaEndpoint,
			Port:       443,
			Protocol:   "https",
			Public:     public,
		}

	}

}

func (m *EndpointsModule) getGrafanaEndPointsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++

	ListWorkspaces, err := sdk.CachedGrafanaListWorkspaces(m.GrafanaClient, aws.ToString(m.Caller.Account), r)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	var public string
	for _, workspace := range ListWorkspaces {
		name := aws.ToString(workspace.Name)
		endpoint := aws.ToString(workspace.Endpoint)
		awsService := "Grafana"

		public = "Unknown"
		protocol := "https"
		var port int32 = 443

		dataReceiver <- Endpoint{
			AWSService: awsService,
			Region:     r,
			Name:       name,
			Endpoint:   endpoint,
			Port:       port,
			Protocol:   protocol,
			Public:     public,
		}

	}

}

func (m *EndpointsModule) getELBv2ListenersPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	// "PaginationMarker" is a control variable used for output continuity, as AWS return the output in pages.
	awsService := "ELBv2"

	LoadBalancers, err := sdk.CachedELBv2DescribeLoadBalancers(m.ELBv2Client, aws.ToString(m.Caller.Account), r)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	var public string
	for _, lb := range LoadBalancers {

		name := aws.ToString(lb.LoadBalancerName)
		arn := aws.ToString(lb.LoadBalancerArn)
		scheme := lb.Scheme

		//TODO: Convert to cacehd function
		ListenerDetails, err := m.ELBv2Client.DescribeListeners(
			context.TODO(),
			&(elasticloadbalancingv2.DescribeListenersInput{
				LoadBalancerArn: &arn,
			}),
			func(o *elasticloadbalancingv2.Options) {
				o.Region = r
			},
		)
		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			continue
		}
		if scheme == "internet-facing" {
			public = "True"
		} else {
			public = "False"
		}

		for _, listener := range ListenerDetails.Listeners {
			endpoint := aws.ToString(lb.DNSName)
			port := aws.ToInt32(listener.Port)
			protocol := string(listener.Protocol)
			if protocol == "HTTPS" {
				endpoint = fmt.Sprintf("https://%s:%s", endpoint, strconv.Itoa(int(port)))
			} else if protocol == "HTTP" {
				endpoint = fmt.Sprintf("http://%s:%s", endpoint, strconv.Itoa(int(port)))
			}

			dataReceiver <- Endpoint{
				AWSService: awsService,
				Region:     r,
				Name:       name,
				Endpoint:   endpoint,
				Port:       port,
				Protocol:   protocol,
				Public:     public,
			}
		}

	}

}

func (m *EndpointsModule) getELBListenersPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	awsService := "ELB"

	LoadBalancerDescriptions, err := sdk.CachedELBDescribeLoadBalancers(m.ELBClient, aws.ToString(m.Caller.Account), r)

	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}
	var public string
	for _, lb := range LoadBalancerDescriptions {

		name := aws.ToString(lb.LoadBalancerName)
		scheme := aws.ToString(lb.Scheme)

		if scheme == "internet-facing" {
			public = "True"
		} else {
			public = "False"
		}

		for _, listener := range lb.ListenerDescriptions {
			endpoint := aws.ToString(lb.DNSName)
			port := listener.Listener.LoadBalancerPort
			protocol := aws.ToString(listener.Listener.Protocol)
			if protocol == "HTTPS" {
				endpoint = fmt.Sprintf("https://%s:%s", endpoint, strconv.Itoa(int(port)))
			} else if protocol == "HTTP" {
				endpoint = fmt.Sprintf("http://%s:%s", endpoint, strconv.Itoa(int(port)))
			}

			dataReceiver <- Endpoint{
				AWSService: awsService,
				Region:     r,
				Name:       name,
				Endpoint:   endpoint,
				Port:       port,
				Protocol:   protocol,
				Public:     public,
			}
		}

	}

}

func (m *EndpointsModule) getAPIGatewayAPIsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	// "PaginationMarker" is a control variable used for output continuity, as AWS return the output in pages.

	Items, err := sdk.CachedApiGatewayGetRestAPIs(m.APIGatewayClient, aws.ToString(m.Caller.Account), r)

	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	for _, api := range Items {
		for _, endpoint := range m.getEndpointsPerAPIGateway(r, api) {
			dataReceiver <- endpoint
		}
	}
}

func (m *EndpointsModule) getAPIGatewayVIPsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++

	Items, err := sdk.CachedApiGatewayGetRestAPIs(m.APIGatewayClient, aws.ToString(m.Caller.Account), r)

	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	GetDomainNames, err := sdk.CachedApiGatewayGetDomainNames(m.APIGatewayClient, aws.ToString(m.Caller.Account), r)

	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, item := range GetDomainNames {

		domain := aws.ToString(item.DomainName)
		GetBasePathMappings, err := sdk.CachedApiGatewayGetBasePathMappings(m.APIGatewayClient, aws.ToString(m.Caller.Account), r, item.DomainName)

		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			break
		}

		for _, mapping := range GetBasePathMappings {
			stage := aws.ToString(mapping.Stage)
			basePath := aws.ToString(mapping.BasePath)
			if basePath == "(none)" {
				basePath = "" // Empty string since '/' is already prepended
			}

			for _, api := range Items {
				if api.Id != nil && aws.ToString(api.Id) == aws.ToString(mapping.RestApiId) {
					endpoints := m.getEndpointsPerAPIGateway(r, api)
					for _, endpoint := range endpoints {
						old := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s/", aws.ToString(mapping.RestApiId), r, stage)

						if strings.HasPrefix(endpoint.Endpoint, old) {
							var new string
							if basePath == "" {
								new = fmt.Sprintf("https://%s/", domain)
							} else {
								new = fmt.Sprintf("https://%s/%s/", domain, basePath)
							}
							endpoint.Endpoint = strings.Replace(endpoint.Endpoint, old, new, 1)
							endpoint.Name = domain
							dataReceiver <- endpoint
						}
					}
					break
				}
			}
		}

	}

}

func (m *EndpointsModule) getEndpointsPerAPIGateway(r string, api apigatewayTypes.RestApi) []Endpoint {
	var endpoints []Endpoint

	awsService := "APIGateway"
	var public string

	name := aws.ToString(api.Name)
	id := aws.ToString(api.Id)
	rawEndpoint := fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com", id, r)
	var port int32 = 443
	protocol := "https"

	endpointType := *api.EndpointConfiguration
	//fmt.Println(endpointType)
	if endpointType.Types[0] == "PRIVATE" {
		public = "False"
	} else {
		public = "True"
	}

	GetStages, err := sdk.CachedApiGatewayGetStages(m.APIGatewayClient, aws.ToString(m.Caller.Account), r, id)

	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	GetResources, err := sdk.CachedApiGatewayGetResources(m.APIGatewayClient, aws.ToString(m.Caller.Account), r, id)

	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, stage := range GetStages.Item {
		stageName := aws.ToString(stage.StageName)
		for _, resource := range GetResources {
			if len(resource.ResourceMethods) != 0 {
				path := aws.ToString(resource.Path)

				endpoint := fmt.Sprintf("%s/%s%s", rawEndpoint, stageName, path)

				endpoints = append(endpoints, Endpoint{
					AWSService: awsService,
					Region:     r,
					Name:       name,
					Endpoint:   endpoint,
					Port:       port,
					Protocol:   protocol,
					Public:     public,
				})
			}
		}
	}

	return endpoints
}

func (m *EndpointsModule) getAPIGatewayv2APIsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	// "PaginationMarker" is a control variable used for output continuity, as AWS return the output in pages.

	Items, err := sdk.CachedAPIGatewayv2GetAPIs(m.APIGatewayv2Client, aws.ToString(m.Caller.Account), r)

	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}
	for _, api := range Items {
		for _, endpoint := range m.getEndpointsPerAPIGatewayv2(r, api) {
			dataReceiver <- endpoint
		}
	}

}

func (m *EndpointsModule) getAPIGatewayv2VIPsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++

	Items, err := sdk.CachedAPIGatewayv2GetAPIs(m.APIGatewayv2Client, aws.ToString(m.Caller.Account), r)

	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	GetDomainNames, err := sdk.CachedAPIGatewayv2GetDomainNames(m.APIGatewayv2Client, aws.ToString(m.Caller.Account), r)

	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, item := range GetDomainNames {

		domain := aws.ToString(item.DomainName)
		GetApiMappings, err := sdk.CachedAPIGatewayv2GetApiMappings(m.APIGatewayv2Client, aws.ToString(m.Caller.Account), r, domain)

		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			break
		}

		for _, mapping := range GetApiMappings {
			stage := aws.ToString(mapping.Stage)
			if stage == "$default" {
				stage = ""
			}
			path := aws.ToString(mapping.ApiMappingKey)

			for _, api := range Items {
				if api.ApiId != nil && aws.ToString(api.ApiId) == aws.ToString(mapping.ApiId) {
					endpoints := m.getEndpointsPerAPIGatewayv2(r, api)
					for _, endpoint := range endpoints {
						var old string
						if stage == "" {
							old = fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/", aws.ToString(mapping.ApiId), r)
						} else {
							old = fmt.Sprintf("https://%s.execute-api.%s.amazonaws.com/%s/", aws.ToString(mapping.ApiId), r, stage)
						}
						if strings.HasPrefix(endpoint.Endpoint, old) {
							var new string
							if path == "" {
								new = fmt.Sprintf("https://%s/", domain)
							} else {
								new = fmt.Sprintf("https://%s/%s/", domain, path)
							}
							endpoint.Endpoint = strings.Replace(endpoint.Endpoint, old, new, 1)
							endpoint.Name = domain
							dataReceiver <- endpoint
						}
					}
					break
				}
			}
		}

	}

}

func (m *EndpointsModule) getEndpointsPerAPIGatewayv2(r string, api apigatewayV2Types.Api) []Endpoint {
	var endpoints []Endpoint
	awsService := "APIGatewayv2"

	var public string

	name := aws.ToString(api.Name)
	rawEndpoint := aws.ToString(api.ApiEndpoint)
	id := aws.ToString(api.ApiId)
	var port int32 = 443
	protocol := "https"

	var stages []string
	GetStages, err := sdk.CachedAPIGatewayv2GetStages(m.APIGatewayv2Client, aws.ToString(m.Caller.Account), r, id)

	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, stage := range GetStages {
		s := aws.ToString(stage.StageName)
		if s == "$default" {
			s = ""
		}
		stages = append(stages, s)
	}

	GetRoutes, err := sdk.CachedAPIGatewayv2GetRoutes(m.APIGatewayv2Client, aws.ToString(m.Caller.Account), r, id)

	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, stage := range stages {
		for _, route := range GetRoutes {
			routeKey := route.RouteKey
			var path string
			if len(strings.Fields(*routeKey)) == 2 {
				path = strings.Fields(*routeKey)[1]
			}
			var endpoint string
			if stage == "" {
				endpoint = fmt.Sprintf("%s%s", rawEndpoint, path)
			} else {
				endpoint = fmt.Sprintf("%s/%s%s", rawEndpoint, stage, path)
			}
			public = "True"

			endpoints = append(endpoints, Endpoint{
				AWSService: awsService,
				Region:     r,
				Name:       name,
				Endpoint:   endpoint,
				Port:       port,
				Protocol:   protocol,
				Public:     public,
			})
		}
	}

	return endpoints
}

func (m *EndpointsModule) getRdsClustersPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++

	DBInstances, err := sdk.CachedRDSDescribeDBInstances(m.RDSClient, aws.ToString(m.Caller.Account), r)
	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	var public string
	for _, instance := range DBInstances {
		if instance.Endpoint != nil {
			name := aws.ToString(instance.DBInstanceIdentifier)
			port := instance.Endpoint.Port
			endpoint := aws.ToString(instance.Endpoint.Address)
			awsService := "RDS"

			if aws.ToBool(instance.PubliclyAccessible) {
				public = "True"
			} else {
				public = "False"
			}

			dataReceiver <- Endpoint{
				AWSService: awsService,
				Region:     r,
				Name:       name,
				Endpoint:   endpoint,
				Port:       aws.ToInt32(port),
				Protocol:   aws.ToString(instance.Engine),
				Public:     public,
			}
		}

	}

}

func (m *EndpointsModule) getRedshiftEndPointsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	awsService := "Redshift"
	protocol := "https"

	// This for loop exits at the end depending on whether the output hits its last page (see pagination control block at the end of the loop).
	Clusters, err := sdk.CachedRedShiftDescribeClusters(m.RedshiftClient, aws.ToString(m.Caller.Account), r)

	if err != nil {
		if errors.As(err, &oe) {
			m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
		}
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	var public string
	for _, cluster := range Clusters {
		name := aws.ToString(cluster.DBName)
		//id := workspace.Id
		endpoint := aws.ToString(cluster.Endpoint.Address)

		if aws.ToBool(cluster.PubliclyAccessible) {
			public = "True"
		} else {
			public = "False"
		}

		port := cluster.Endpoint.Port

		dataReceiver <- Endpoint{
			AWSService: awsService,
			Region:     r,
			Name:       name,
			Endpoint:   endpoint,
			Port:       aws.ToInt32(port),
			Protocol:   protocol,
			Public:     public,
		}

	}

}

/*
UNUSED CODE - PLEASE REVIEW AND DELETE IF IT DOESN'T APPLY

	func (m *EndpointsModule) getS3EndpointsPerRegion(wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
		defer func() {
			m.CommandCounter.Executing--
			m.CommandCounter.Complete++
			wg.Done()

		}()
		semaphore <- struct{}{}
		defer func() {
			<-semaphore
		}()
		// m.CommandCounter.Total++
		m.CommandCounter.Pending--
		m.CommandCounter.Executing++

		// This for loop exits at the end dependeding on whether the output hits its last page (see pagination control block at the end of the loop).
		ListBuckets, _ := m.S3Client.ListBuckets(
			context.TODO(),
			&s3.ListBucketsInput{},
		)

		var public string
		for _, bucket := range ListBuckets.Buckets {
			name := aws.ToString(bucket.Name)
			endpoint := fmt.Sprintf("https://%s.s3.amazonaws.com", name)
			awsService := "S3"

			var port int32 = 443
			protocol := "https"
			var r string = "Global"
			public = "False"

			GetBucketPolicyStatus, err := m.S3Client.GetBucketPolicyStatus(
				context.TODO(),
				&s3.GetBucketPolicyStatusInput{
					Bucket: &name,
				},
			)

			if err == nil {
				isPublic := GetBucketPolicyStatus.PolicyStatus.IsPublic
				if isPublic {
					public = "True"
				}
			}

			// GetBucketWebsite, err := m.S3Client.GetBucketWebsite(
			// 	context.TODO(),
			// 	&s3.GetBucketWebsiteInput{
			// 		Bucket: &name,
			// 	},
			// )

			// if err != nil {
			// 	index := *GetBucketWebsite.IndexDocument.Suffix
			// 	if index != "" {
			// 		public = "True"
			// 	}

			// }

			dataReceiver <- Endpoint{
				AWSService: awsService,
				Region:     r,
				Name:       name,
				Endpoint:   endpoint,
				Port:       port,
				Protocol:   protocol,
				Public:     public,
			}

		}
	}
*/
func (m *EndpointsModule) getCloudfrontEndpoints(wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	// "PaginationMarker" is a control variable used for output continuity, as AWS return the output in pages.
	var PaginationControl *string
	var awsService = "Cloudfront"
	var protocol = "https"
	var r = "Global"
	var public = "True"

	// This for loop exits at the end depending on whether the output hits its last page (see pagination control block at the end of the loop).
	for {
		ListDistributions, err := m.CloudfrontClient.ListDistributions(
			context.TODO(),
			&cloudfront.ListDistributionsInput{
				Marker: PaginationControl,
			},
		)
		if err != nil {
			if errors.As(err, &oe) {
				m.Errors = append(m.Errors, fmt.Sprintf(" Error: Region: %s, Service: %s, Operation: %s", r, oe.Service(), oe.Operation()))
			}
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			break
		}
		if ListDistributions.DistributionList.Quantity == nil {
			break
		}
		// var public string
		// var hostnames []string
		// var aliases []string
		// var origins []string

		for _, item := range ListDistributions.DistributionList.Items {
			name := aws.ToString(item.DomainName)
			public = "True"
			var port int32 = 443
			endpoint := fmt.Sprintf("https://%s", aws.ToString(item.DomainName))
			dataReceiver <- Endpoint{
				AWSService: awsService,
				Region:     r,
				Name:       name,
				Endpoint:   endpoint,
				Port:       port,
				Protocol:   protocol,
				Public:     public,
			}
			//fmt.Println(*item.DomainName)
			for _, alias := range item.Aliases.Items {
				//aliases = append(aliases, alias)

				endpoint := fmt.Sprintf("https://%s", alias)
				awsServiceAlias := fmt.Sprintf("%s [alias]", awsService)
				dataReceiver <- Endpoint{
					AWSService: awsServiceAlias,
					Region:     r,
					Name:       name,
					Endpoint:   endpoint,
					Port:       port,
					Protocol:   protocol,
					Public:     public,
				}
			}

			for _, origin := range item.Origins.Items {
				//origins = append(origins, *origin.DomainName)
				//fmt.Println(origin.DomainName)
				public = "Unknown"
				var port int32 = 443
				path := aws.ToString(origin.OriginPath)
				if !strings.HasPrefix(path, "/") {
					path = "/" + path
				}
				endpoint := fmt.Sprintf("https://%s%s", aws.ToString(origin.DomainName), path)
				awsServiceOrigin := fmt.Sprintf("%s [origin]", awsService)
				dataReceiver <- Endpoint{
					AWSService: awsServiceOrigin,
					Region:     r,
					Name:       name,
					Endpoint:   endpoint,
					Port:       port,
					Protocol:   protocol,
					Public:     public,
				}
			}

		}

		// port := cluster.Endpoint.Port

		// Pagination control. After the last page of output, the for loop exits.
		if ListDistributions.DistributionList.NextMarker != nil {
			PaginationControl = ListDistributions.DistributionList.NextMarker
		} else {
			PaginationControl = nil
			break
		}
	}
}

func (m *EndpointsModule) getAppRunnerEndpointsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++

	ServiceSummaryList, err := sdk.CachedAppRunnerListServices(m.AppRunnerClient, aws.ToString(m.Caller.Account), r)

	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}
	for _, service := range ServiceSummaryList {

		endpoint := &Endpoint{
			AWSService: "App Runner",
		}

		endpoint.Name = aws.ToString(service.ServiceName)
		endpoint.Port = 443
		endpoint.Protocol = "https"
		endpoint.Region = r

		arn := aws.ToString(service.ServiceArn)

		DescribeService, err := m.AppRunnerClient.DescribeService(
			context.TODO(),
			&apprunner.DescribeServiceInput{
				ServiceArn: &arn,
			},
			func(o *apprunner.Options) {
				o.Region = r
			},
		)
		if err != nil {
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			break
		}

		if DescribeService.Service.NetworkConfiguration.IngressConfiguration.IsPubliclyAccessible {
			endpoint.Public = "True"
		} else {
			endpoint.Public = "False"
		}

		if service.ServiceUrl != nil {
			endpoint.Endpoint = fmt.Sprintf("https://%s", aws.ToString(service.ServiceUrl))
		} else {
			DescribeCustomDomains, err := m.AppRunnerClient.DescribeCustomDomains(
				context.TODO(),
				&apprunner.DescribeCustomDomainsInput{
					ServiceArn: &arn,
				},
				func(o *apprunner.Options) {
					o.Region = r
				},
			)
			if err != nil {
				m.modLog.Error(err.Error())
				m.CommandCounter.Error++
				break
			}
			if DescribeCustomDomains.DNSTarget != nil {
				endpoint.Endpoint = fmt.Sprintf("https://%s", aws.ToString(DescribeCustomDomains.DNSTarget))
			} else {
				endpoint.Endpoint = "Unknown"

			}
		}

		dataReceiver <- *endpoint

	}
}

func (m *EndpointsModule) appRunnerDescribeCustomDomain(r string, serviceArn string) ([]apprunnerTypes.CustomDomain, error) {
	var PaginationControl *string
	var domains []apprunnerTypes.CustomDomain
	for {
		ListDomains, err := m.AppRunnerClient.DescribeCustomDomains(
			context.TODO(),
			&(apprunner.DescribeCustomDomainsInput{
				ServiceArn: &serviceArn,
				NextToken:  PaginationControl,
			}),
			func(o *apprunner.Options) {
				o.Region = r
			},
		)
		if err != nil {
			return domains, err
		}
		if len(ListDomains.CustomDomains) > 0 {
			for _, domain := range ListDomains.CustomDomains {
				domains = append(domains, domain)
			}
		}

		// The "NextToken" value is nil when there's no more data to return.
		if ListDomains.NextToken != nil {
			PaginationControl = ListDomains.NextToken
		} else {
			PaginationControl = nil
			break
		}
	}
	return domains, nil

}

func (m *EndpointsModule) getLightsailContainerEndpointsPerRegion(r string, wg *sync.WaitGroup, semaphore chan struct{}, dataReceiver chan Endpoint) {
	defer func() {
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
		wg.Done()

	}()
	semaphore <- struct{}{}
	defer func() {
		<-semaphore
	}()
	// m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	var public = "True"
	var protocol = "https"
	var port int32 = 443

	containerServices, err := sdk.CachedLightsailGetContainerServices(m.LightsailClient, aws.ToString(m.Caller.Account), r)

	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}
	if len(containerServices) > 0 {

		for _, containerService := range containerServices {
			name := aws.ToString(containerService.ContainerServiceName)
			endpoint := aws.ToString(containerService.Url)
			awsService := "Lightsail [Container]"

			dataReceiver <- Endpoint{
				AWSService: awsService,
				Region:     r,
				Name:       name,
				Endpoint:   endpoint,
				Port:       port,
				Protocol:   protocol,
				Public:     public,
			}
		}
	}
}
