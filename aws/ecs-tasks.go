package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/BishopFox/cloudfox/aws/sdk"
	"github.com/BishopFox/cloudfox/internal"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/bishopfox/awsservicemap"
	"github.com/sirupsen/logrus"
)

type ECSTasksModule struct {
	ECSClient sdk.AWSECSClientInterface
	EC2Client sdk.AWSEC2ClientInterface
	IAMClient sdk.AWSIAMClientInterface

	Caller              sts.GetCallerIdentityOutput
	AWSRegions          []string
	AWSOutputType       string
	AWSTableCols        string
	PmapperDataBasePath string

	AWSProfile     string
	Goroutines     int
	SkipAdminCheck bool
	WrapTable      bool
	pmapperMod     PmapperModule
	pmapperError   error
	iamSimClient   IamSimulatorModule

	MappedECSTasks []MappedECSTask
	CommandCounter internal.CommandCounter

	output internal.OutputData2
	modLog *logrus.Entry
}

type MappedECSTask struct {
	Cluster               string
	TaskDefinitionName    string
	TaskDefinitionContent string
	ContainerName         string
	LaunchType            string
	ID                    string
	ExternalIP            string
	PrivateIP             string
	Role                  string
	Admin                 string
	CanPrivEsc            string
}

func (m *ECSTasksModule) ECSTasks(outputDirectory string, verbosity int) {
	m.output.Verbosity = verbosity
	m.output.Directory = outputDirectory
	m.output.CallingModule = "ecs-tasks"
	localAdminMap := make(map[string]bool)
	m.modLog = internal.TxtLog.WithFields(logrus.Fields{
		"module": m.output.CallingModule,
	})
	if m.AWSProfile == "" {
		m.AWSProfile = internal.BuildAWSPath(m.Caller)
	}

	fmt.Printf("[%s][%s] Enumerating ECS tasks in all regions for account %s\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), aws.ToString(m.Caller.Account))
	// Initialized the tools we'll need to check if any workload roles are admin or can privesc to admin
	//fmt.Printf("[%s][%s] Attempting to build a PrivEsc graph in memory using local pmapper data if it exists on the filesystem.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile))
	m.pmapperMod, m.pmapperError = InitPmapperGraph(m.Caller, m.AWSProfile, m.Goroutines, m.PmapperDataBasePath)
	m.iamSimClient = InitIamCommandClient(m.IAMClient, m.Caller, m.AWSProfile, m.Goroutines)

	// if m.pmapperError != nil {
	// 	fmt.Printf("[%s][%s] No pmapper data found for this account. Using cloudfox's iam-simulator for role analysis.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile))
	// } else {
	// 	fmt.Printf("[%s][%s] Found pmapper data for this account. Using it for role analysis.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile))
	// }
	fmt.Printf("[%s][%s] For context and next steps: https://github.com/BishopFox/cloudfox/wiki/AWS-Commands#%s\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), m.output.CallingModule)

	wg := new(sync.WaitGroup)

	spinnerDone := make(chan bool)
	go internal.SpinUntil(m.output.CallingModule, &m.CommandCounter, spinnerDone, "tasks")

	dataReceiver := make(chan MappedECSTask)

	// Create a channel to signal to stop
	receiverDone := make(chan bool)

	go m.Receiver(dataReceiver, receiverDone)

	for _, region := range m.AWSRegions {
		wg.Add(1)
		m.CommandCounter.Pending++
		go m.executeChecks(region, wg, dataReceiver)

	}

	wg.Wait()
	//time.Sleep(time.Second * 2)

	// Perform role analysis
	if m.pmapperError == nil {
		for i := range m.MappedECSTasks {
			m.MappedECSTasks[i].Admin, m.MappedECSTasks[i].CanPrivEsc = GetPmapperResults(m.SkipAdminCheck, m.pmapperMod, &m.MappedECSTasks[i].Role)
		}
	} else {
		for i := range m.MappedECSTasks {
			m.MappedECSTasks[i].Admin, m.MappedECSTasks[i].CanPrivEsc = GetIamSimResult(m.SkipAdminCheck, &m.MappedECSTasks[i].Role, m.iamSimClient, localAdminMap)
		}
	}

	spinnerDone <- true
	<-spinnerDone
	receiverDone <- true
	<-receiverDone

	m.printECSTaskData(outputDirectory, dataReceiver, verbosity)

}

func (m *ECSTasksModule) Receiver(receiver chan MappedECSTask, receiverDone chan bool) {
	defer close(receiverDone)
	for {
		select {
		case data := <-receiver:
			m.MappedECSTasks = append(m.MappedECSTasks, data)
		case <-receiverDone:
			receiverDone <- true
			return
		}
	}
}

func (m *ECSTasksModule) printECSTaskData(outputDirectory string, dataReceiver chan MappedECSTask, verbosity int) {
	// This is the complete list of potential table columns
	m.output.Headers = []string{
		"Account",
		"Cluster",
		"TaskDefinition",
		"ContainerName",
		"LaunchType",
		"ID",
		"External IP",
		"Internal IP",
		"RoleArn",
		"IsAdminRole?",
		"CanPrivEscToAdmin?",
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
			"Cluster",
			"TaskDefinition",
			"ContainerName",
			"LaunchType",
			"ID",
			"External IP",
			"Internal IP",
			"RoleArn",
			"IsAdminRole?",
			"CanPrivEscToAdmin?",
		}
		// Otherwise, use the default columns.
	} else {
		tableCols = []string{
			"Cluster",
			"TaskDefinition",
			"ContainerName",
			"LaunchType",
			"External IP",
			"Internal IP",
			"RoleArn",
			"IsAdminRole?",
			"CanPrivEscToAdmin?",
		}
	}

	// Remove the pmapper row if there is no pmapper data
	if m.pmapperError != nil {
		sharedLogger.Errorf("%s - %s - No pmapper data found for this account. Skipping the pmapper column in the output table.", m.output.CallingModule, m.AWSProfile)
		tableCols = removeStringFromSlice(tableCols, "CanPrivEscToAdmin?")
	}

	for _, ecsTask := range m.MappedECSTasks {
		m.output.Body = append(
			m.output.Body,
			[]string{
				aws.ToString(m.Caller.Account),
				ecsTask.Cluster,
				ecsTask.TaskDefinitionName,
				ecsTask.ContainerName,
				ecsTask.LaunchType,
				ecsTask.ID,
				ecsTask.ExternalIP,
				ecsTask.PrivateIP,
				ecsTask.Role,
				ecsTask.Admin,
				ecsTask.CanPrivEsc,
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
		m.writeLoot(o.Table.DirectoryName)
		fmt.Printf("[%s][%s] %s ECS tasks found.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), strconv.Itoa(len(m.output.Body)))

	} else {
		fmt.Printf("[%s][%s] No ECS tasks found, skipping the creation of an output file.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile))
	}
}

func (m *ECSTasksModule) writeLoot(outputDirectory string) {
	path := filepath.Join(outputDirectory, "loot")
	err := os.MkdirAll(path, os.ModePerm)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}
	privateIPsFilename := filepath.Join(path, "ecs-tasks-PrivateIPs.txt")
	publicIPsFilename := filepath.Join(path, "ecs-tasks-PublicIPs.txt")

	var publicIPs string
	var privateIPs string

	for _, task := range m.MappedECSTasks {
		if task.ExternalIP != "NoExternalIP" {
			publicIPs = publicIPs + fmt.Sprintln(task.ExternalIP)
		}
		if task.PrivateIP != "" {
			privateIPs = privateIPs + fmt.Sprintln(task.PrivateIP)
		}

	}
	err = os.WriteFile(privateIPsFilename, []byte(privateIPs), 0644)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}
	err = os.WriteFile(publicIPsFilename, []byte(publicIPs), 0644)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, task := range m.MappedECSTasks {
		if task.TaskDefinitionContent != "" {
			path := filepath.Join(path, "task-definitions")
			err := os.MkdirAll(path, os.ModePerm)
			if err != nil {
				m.modLog.Error(err.Error())
				m.CommandCounter.Error++
			}
			taskDefinitionFilename := filepath.Join(path, task.TaskDefinitionName+".json")

			err = os.WriteFile(taskDefinitionFilename, []byte(task.TaskDefinitionContent), 0644)
			if err != nil {
				m.modLog.Error(err.Error())
				m.CommandCounter.Error++
			}
		}
	}

	fmt.Printf("[%s][%s] Loot written to [%s]\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), privateIPsFilename)
	fmt.Printf("[%s][%s] Loot written to [%s]\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), publicIPsFilename)

}

func (m *ECSTasksModule) executeChecks(r string, wg *sync.WaitGroup, dataReceiver chan MappedECSTask) {
	defer wg.Done()

	servicemap := &awsservicemap.AwsServiceMap{
		JsonFileSource: "DOWNLOAD_FROM_AWS",
	}
	res, err := servicemap.IsServiceInRegion("ecs", r)
	if err != nil {
		m.modLog.Error(err)
	}
	if res {

		m.CommandCounter.Total++
		m.CommandCounter.Pending--
		m.CommandCounter.Executing++
		m.getListClusters(r, dataReceiver)
		m.CommandCounter.Executing--
		m.CommandCounter.Complete++
	}
}

func (m *ECSTasksModule) getListClusters(region string, dataReceiver chan MappedECSTask) {

	ClusterArns, err := sdk.CachedECSListClusters(m.ECSClient, aws.ToString(m.Caller.Account), region)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	for _, clusterARN := range ClusterArns {
		m.getListTasks(clusterARN, region, dataReceiver)
	}

}

func (m *ECSTasksModule) getListTasks(clusterARN string, region string, dataReceiver chan MappedECSTask) {
	TaskArns, err := sdk.CachedECSListTasks(m.ECSClient, aws.ToString(m.Caller.Account), region, clusterARN)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	batchSize := 100 // maximum value: https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_DescribeTasks.html#API_DescribeTasks_RequestSyntax
	for i := 0; i < len(TaskArns); i += batchSize {
		j := i + batchSize
		if j > len(TaskArns) {
			j = len(TaskArns)
		}

		m.loadTasksData(clusterARN, TaskArns[i:j], region, dataReceiver)
	}

}

func (m *ECSTasksModule) loadTasksData(clusterARN string, taskARNs []string, region string, dataReceiver chan MappedECSTask) {

	if len(taskARNs) == 0 {
		return
	}

	Tasks, err := sdk.CachedECSDescribeTasks(m.ECSClient, aws.ToString(m.Caller.Account), region, clusterARN, taskARNs)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	var eniIDs []string
	for _, task := range Tasks {
		eniID := getElasticNetworkInterfaceIDOfECSTask(task)
		if eniID != "" {
			eniIDs = append(eniIDs, eniID)
		}
	}
	publicIPs, err := m.loadPublicIPs(eniIDs, region)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return
	}

	for _, task := range Tasks {
		//taskDefinition, err := m.describeTaskDefinition(aws.ToString(task.TaskDefinitionArn), region)
		taskDefinition, err := sdk.CachedECSDescribeTaskDefinition(m.ECSClient, aws.ToString(m.Caller.Account), region, aws.ToString(task.TaskDefinitionArn))
		if err != nil {
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			return
		}
		mappedTask := MappedECSTask{
			Cluster:               getNameFromARN(clusterARN),
			TaskDefinitionName:    getNameFromARN(aws.ToString(task.TaskDefinitionArn)),
			TaskDefinitionContent: getTaskDefinitionContent(taskDefinition),
			ContainerName:         getContainerNamesFromECSTask(task),
			LaunchType:            string(task.LaunchType),
			ID:                    getIDFromECSTask(aws.ToString(task.TaskArn)),
			PrivateIP:             getPrivateIPv4AddressFromECSTask(task),
			Role:                  getTaskRole(taskDefinition),
		}

		eniID := getElasticNetworkInterfaceIDOfECSTask(task)
		if eniID != "" {
			mappedTask.ExternalIP = publicIPs[eniID]
		}

		dataReceiver <- mappedTask
	}
}

func getTaskRole(taskDefinition types.TaskDefinition) string {
	return aws.ToString(taskDefinition.TaskRoleArn)
}

func getTaskDefinitionContent(taskDefinition types.TaskDefinition) string {
	// return taskDefinition as a json string

	taskDefinitionContent, err := json.Marshal(taskDefinition)
	if err != nil {
		return ""
	}
	return string(taskDefinitionContent)
}

func (m *ECSTasksModule) describeTaskDefinition(taskDefinitionArn string, region string) (types.TaskDefinition, error) {
	DescribeTaskDefinition, err := m.ECSClient.DescribeTaskDefinition(
		context.TODO(),
		&ecs.DescribeTaskDefinitionInput{
			TaskDefinition: &taskDefinitionArn,
		},
		func(o *ecs.Options) {
			o.Region = region
		},
	)
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
		return types.TaskDefinition{}, err
	}
	return *DescribeTaskDefinition.TaskDefinition, nil
}

/* UNUSED CODE BLOCK - PLEASE REVIEW AND DELETE IF NOT NEEDED
func (m *ECSTasksModule) loadAllPublicIPs(eniIDs []string, region string) (map[string]string, error) {
	eniPublicIPs := make(map[string]string)

	batchSize := 1000 // seems to be maximum value: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeNetworkInterfaces.html
	for i := 0; i < len(eniIDs); i += batchSize {
		j := i + batchSize
		if j > len(eniIDs) {
			j = len(eniIDs)
		}

		publicIPs, err := m.loadPublicIPs(eniIDs[i:j], region)
		if err != nil {
			m.modLog.Error(err.Error())
			m.CommandCounter.Error++
			return nil, fmt.Errorf("getting elastic network interfaces: %s", err)
		}

		for eniID, publicIP := range publicIPs {
			eniPublicIPs[eniID] = publicIP
		}
	}

	return eniPublicIPs, nil
}
*/

func (m *ECSTasksModule) loadPublicIPs(eniIDs []string, region string) (map[string]string, error) {
	eniPublicIPs := make(map[string]string)

	if len(eniIDs) == 0 {
		return eniPublicIPs, nil
	}

	NetworkInterfaces, err := sdk.CachedEC2DescribeNetworkInterfaces(m.EC2Client, aws.ToString(m.Caller.Account), region)
	if err != nil {
		return nil, fmt.Errorf("getting elastic network interfaces: %s", err)
	}

	for _, eni := range NetworkInterfaces {
		eniPublicIPs[aws.ToString(eni.NetworkInterfaceId)] = getPublicIPOfElasticNetworkInterface(eni)
	}

	return eniPublicIPs, nil
}

func getNameFromARN(arn string) string {
	tokens := strings.SplitN(arn, "/", 2)
	if len(tokens) != 2 {
		return arn
	}

	return tokens[1]
}

func getIDFromECSTask(arn string) string {
	tokens := strings.SplitN(arn, "/", 3)
	if len(tokens) != 3 {
		return arn
	}

	return tokens[2]
}

func getContainerNamesFromECSTask(task types.Task) string {
	var names []string

	for _, container := range task.Containers {
		names = append(names, aws.ToString(container.Name))
	}

	return strings.Join(names, "|")
}

func getPrivateIPv4AddressFromECSTask(task types.Task) string {
	var ips []string

	for _, attachment := range task.Attachments {
		if aws.ToString(attachment.Type) != "ElasticNetworkInterface" || aws.ToString(attachment.Status) != "ATTACHED" {
			continue
		}

		for _, kvp := range attachment.Details {
			if aws.ToString(kvp.Name) == "privateIPv4Address" {
				ips = append(ips, aws.ToString(kvp.Value))
			}
		}
	}

	return strings.Join(ips, "|")
}

func getElasticNetworkInterfaceIDOfECSTask(task types.Task) string {
	for _, attachment := range task.Attachments {
		if aws.ToString(attachment.Type) != "ElasticNetworkInterface" || aws.ToString(attachment.Status) != "ATTACHED" {
			continue
		}

		for _, kvp := range attachment.Details {
			if aws.ToString(kvp.Name) == "networkInterfaceId" {
				return aws.ToString(kvp.Value)
			}
		}
	}

	return ""
}
