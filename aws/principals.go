package aws

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BishopFox/cloudfox/aws/sdk"
	"github.com/BishopFox/cloudfox/internal"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/sirupsen/logrus"
)

type IamPrincipalsModule struct {
	// General configuration data
	IAMClient sdk.AWSIAMClientInterface

	Caller        sts.GetCallerIdentityOutput
	AWSRegions    []string
	AWSOutputType string
	AWSTableCols  string

	Goroutines int
	AWSProfile string
	WrapTable  bool

	SkipAdminCheck      bool
	iamSimClient        IamSimulatorModule
	pmapperMod          PmapperModule
	pmapperError        error
	PmapperDataBasePath string

	// Main module data
	Users          []User
	Roles          []Role
	Groups         []Group
	CommandCounter internal.CommandCounter
	// Used to store output data for pretty printing
	output internal.OutputData2
	modLog *logrus.Entry
}

type User struct {
	AWSService       string
	Type             string
	Arn              string
	Name             string
	AttachedPolicies []string
	InlinePolicies   []string
	Admin            string
	CanPrivEsc       string
}

type Group struct {
	AWSService       string
	Type             string
	Arn              string
	Name             string
	AttachedPolicies []string
	InlinePolicies   []string
	AttachedUsers    []string
}

type Role struct {
	AWSService       string
	Type             string
	Arn              string
	Name             string
	AttachedPolicies []string
	InlinePolicies   []string
	Admin            string
	CanPrivEsc       string
}

func (m *IamPrincipalsModule) PrintIamPrincipals(outputDirectory string, verbosity int) {
	// These struct values are used by the output module
	m.output.Verbosity = verbosity
	m.output.Directory = outputDirectory
	m.output.CallingModule = "principals"
	localAdminMap := make(map[string]bool)
	m.modLog = internal.TxtLog.WithFields(logrus.Fields{
		"module": m.output.CallingModule,
	})
	if m.AWSProfile == "" {
		m.AWSProfile = internal.BuildAWSPath(m.Caller)
	}

	fmt.Printf("[%s][%s] Enumerating IAM Users and Roles for account %s.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), aws.ToString(m.Caller.Account))

	m.pmapperMod, m.pmapperError = InitPmapperGraph(m.Caller, m.AWSProfile, m.Goroutines, m.PmapperDataBasePath)
	m.iamSimClient = InitIamCommandClient(m.IAMClient, m.Caller, m.AWSProfile, m.Goroutines)

	// wg := new(sync.WaitGroup)

	// done := make(chan bool)
	// go internal.SpinUntil(m.output.CallingModule, &m.CommandCounter, done)
	// wg.Add(1)
	// m.CommandCounter.Pending++
	//m.executeChecks(wg)
	// wg.Wait()
	// done <- true
	// <-done

	m.addIAMUsersToTable()
	m.addIAMRolesToTable()

	//fmt.Printf("\nAnalyzed Resources by Region\n\n")

	m.output.Headers = []string{
		"Account",
		"Type",
		"Name",
		"Arn",
		"AttachedPolicies",
		"InlinePolicies",
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
	} else if m.AWSOutputType == "wide" {
		tableCols = []string{
			"Account",
			"Type",
			"Name",
			"Arn",
			//"AttachedPolicies",
			//"InlinePolicies",
			"IsAdminRole?",
			"CanPrivEscToAdmin?",
		}

		// Otherwise, use the default columns.
	} else {
		tableCols = []string{
			"Type",
			"Name",
			"Arn",
			// "AttachedPolicies",
			// "InlinePolicies",
			"IsAdminRole?",
			"CanPrivEscToAdmin?",
		}
	}

	// Remove the pmapper row if there is no pmapper data
	if m.pmapperError != nil {
		sharedLogger.Errorf("%s - %s - No pmapper data found for this account. Skipping the pmapper column in the output table.", m.output.CallingModule, m.AWSProfile)
		tableCols = removeStringFromSlice(tableCols, "CanPrivEscToAdmin?")
	}

	//Table rows
	for i := range m.Users {
		if m.pmapperError == nil {
			m.Users[i].Admin, m.Users[i].CanPrivEsc = GetPmapperResults(m.SkipAdminCheck, m.pmapperMod, &m.Users[i].Arn)
		} else {
			m.Users[i].Admin, m.Users[i].CanPrivEsc = GetIamSimResult(m.SkipAdminCheck, &m.Users[i].Arn, m.iamSimClient, localAdminMap)
		}

		m.output.Body = append(
			m.output.Body,
			[]string{
				aws.ToString(m.Caller.Account),
				m.Users[i].Type,
				m.Users[i].Name,
				m.Users[i].Arn,
				strings.Join(m.Users[i].AttachedPolicies, " , "),
				strings.Join(m.Users[i].InlinePolicies, " , "),
				m.Users[i].Admin,
				m.Users[i].CanPrivEsc,
			},
		)

	}

	for i := range m.Roles {
		if m.pmapperError == nil {
			m.Roles[i].Admin, m.Roles[i].CanPrivEsc = GetPmapperResults(m.SkipAdminCheck, m.pmapperMod, &m.Roles[i].Arn)
		} else {
			m.Roles[i].Admin, m.Roles[i].CanPrivEsc = GetIamSimResult(m.SkipAdminCheck, &m.Roles[i].Arn, m.iamSimClient, localAdminMap)
		}
		m.output.Body = append(
			m.output.Body,
			[]string{
				aws.ToString(m.Caller.Account),
				m.Roles[i].Type,
				m.Roles[i].Name,
				m.Roles[i].Arn,
				strings.Join(m.Roles[i].AttachedPolicies, " , "),
				strings.Join(m.Roles[i].InlinePolicies, " , "),
				m.Roles[i].Admin,
				m.Roles[i].CanPrivEsc,
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
		//m.writeLoot(o.Table.DirectoryName, verbosity)
		fmt.Printf("[%s][%s] %s IAM principals found.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), strconv.Itoa(len(m.output.Body)))

	} else {
		fmt.Printf("[%s][%s] No IAM principals found, skipping the creation of an output file.\n", cyan(m.output.CallingModule), cyan(m.AWSProfile))
	}
	fmt.Printf("[%s][%s] For context and next steps: https://github.com/BishopFox/cloudfox/wiki/AWS-Commands#%s\n", cyan(m.output.CallingModule), cyan(m.AWSProfile), m.output.CallingModule)
}

/* UNUSED CODE BLOCK - PLEASE REVIEW AND DELETE IF APPLICABLE
func (m *IamPrincipalsModule) executeChecks(wg *sync.WaitGroup) {
	defer wg.Done()
	m.CommandCounter.Total++
	m.CommandCounter.Pending--
	m.CommandCounter.Executing++
	m.getIAMUsers()
	m.getIAMRoles()
	m.CommandCounter.Executing--
	m.CommandCounter.Complete++
}
*/

func (m *IamPrincipalsModule) addIAMUsersToTable() {
	var AWSService = "IAM"
	var IAMtype = "User"
	var attachedPolicies []string
	var inlinePolicies []string

	ListUsers, err := sdk.CachedIamListUsers(m.IAMClient, aws.ToString(m.Caller.Account))
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, user := range ListUsers {
		arn := user.Arn
		name := user.UserName

		m.Users = append(
			m.Users,
			User{
				AWSService:       AWSService,
				Arn:              aws.ToString(arn),
				Name:             aws.ToString(name),
				Type:             IAMtype,
				AttachedPolicies: attachedPolicies,
				InlinePolicies:   inlinePolicies,
			})
	}

}

func (m *IamPrincipalsModule) addIAMRolesToTable() {

	//var totalRoles int
	var AWSService = "IAM"
	var IAMtype = "Role"
	var attachedPolicies []string
	var inlinePolicies []string

	ListRoles, err := sdk.CachedIamListRoles(m.IAMClient, aws.ToString(m.Caller.Account))
	if err != nil {
		m.modLog.Error(err.Error())
		m.CommandCounter.Error++
	}

	for _, role := range ListRoles {
		arn := role.Arn
		name := role.RoleName

		m.Roles = append(
			m.Roles,
			Role{
				AWSService:       AWSService,
				Arn:              aws.ToString(arn),
				Name:             aws.ToString(name),
				Type:             IAMtype,
				AttachedPolicies: attachedPolicies,
				InlinePolicies:   inlinePolicies,
			})
	}

}
