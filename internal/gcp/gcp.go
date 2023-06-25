package gcp

import (
	"context"
	"log"
	"os"
	"fmt"
	"bufio"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	goauth2 "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/cloudresourcemanager/v3"
	"google.golang.org/api/cloudasset/v1p1beta1"
	"github.com/BishopFox/cloudfox/internal"
)

var (
	logger = internal.NewLogger()
)

type GCPClient struct {
	Logger internal.Logger
	TokenSource *oauth2.TokenSource
	TokenInfo *goauth2.Tokeninfo
	CloudresourcemanagerService *cloudresourcemanager.Service
	OrganizationsService *cloudresourcemanager.OrganizationsService
	FoldersService *cloudresourcemanager.FoldersService
	ProjectsService *cloudresourcemanager.ProjectsService
	CloudAssetService *cloudasset.Service
	ResourcesService *cloudasset.ResourcesService
	IamPoliciesService *cloudasset.IamPoliciesService
}

func (g *GCPClient) init() {
	g.Logger = internal.NewLogger()
	ctx := context.Background()
	var (
		profiles []GCloudProfile
		profile GCloudProfile
	)
	profiles = listAllProfiles()
	profile = profiles[len(profiles)-1]

	// Initiate an http.Client. The following GET request will be
	// authorized and authenticated on the behalf of the SDK user.
	client := profile.oauth_conf.Client(ctx, &(profile.initial_token))
	ts, err := google.DefaultTokenSource(ctx)
	if err != nil {
		log.Fatal(err)
	}
	g.TokenSource = &ts
	oauth2Service, err := goauth2.NewService(ctx, option.WithHTTPClient(client))
	tokenInfo, err := oauth2Service.Tokeninfo().Do()
	if err != nil {
		log.Fatal(err)
	}
	g.TokenInfo = tokenInfo
	cloudresourcemanagerService, err := cloudresourcemanager.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatal(err)
	}
	g.CloudresourcemanagerService = cloudresourcemanagerService
	cloudassetService, err := cloudasset.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		log.Fatal(err)
	}
	g.CloudAssetService = cloudassetService
	g.ResourcesService = cloudasset.NewResourcesService(cloudassetService)
	g.IamPoliciesService = cloudasset.NewIamPoliciesService(cloudassetService)
	g.OrganizationsService = cloudresourcemanager.NewOrganizationsService(cloudresourcemanagerService)
	g.FoldersService = cloudresourcemanager.NewFoldersService(cloudresourcemanagerService)
	g.ProjectsService = cloudresourcemanager.NewProjectsService(cloudresourcemanagerService)
	
}

func NewGCPClient() *GCPClient {
	client := new(GCPClient)
	client.init()
	return client
}
/*
	Get all usable GCP Profiles
	We are using only non expired user-tokens
*/
func GetAllGCPProfiles() []string {
	var (
		GCPProfiles []string
		accessTokens []Token
	)
	accessTokens = ReadAccessTokens()
	for _, accessToken := range accessTokens {
		if (!strings.Contains(accessToken.AccountID, "@")) {
			continue
		}

		exp, _ := time.Parse(time.RFC3339, accessToken.TokenExpiry)
		if exp.After(time.Now()) {
			GCPProfiles = append(GCPProfiles, accessToken.AccountID)
		}
	}
	return GCPProfiles
}

func ConfirmSelectedProfiles(GCPProfiles []string) bool {
	reader := bufio.NewReader(os.Stdin)
	logger.Info("Identified profiles:\n")
	for _, profile := range GCPProfiles {
		fmt.Printf("\t* %s\n", profile)
	}
	fmt.Printf("\n")
	logger.Info(fmt.Sprintf("Are you sure you'd like to run this command against the [%d] listed profile(s)? (Y\\n): ", len(GCPProfiles)))
	text, _ := reader.ReadString('\n')
	switch text {
	case "\n", "Y\n", "y\n":
		return true
	}
	return false

}

func GetSelectedGCPProfiles(GCPProfilesListPath string) []string {
	GCPProfilesListFile, err := internal.UtilsFs.Open(GCPProfilesListPath)
	internal.CheckErr(err, fmt.Sprintf("could not open given file %s", GCPProfilesListPath))
	if err != nil {
		fmt.Printf("\nError loading profiles. Could not open file at location[%s]\n", GCPProfilesListPath)
		os.Exit(1)
	}
	defer GCPProfilesListFile.Close()
	var GCPProfiles []string
	scanner := bufio.NewScanner(GCPProfilesListFile)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		profile := strings.TrimSpace(scanner.Text())
		if len(profile) != 0 {
			GCPProfiles = append(GCPProfiles, profile)
		}
	}
	return GCPProfiles
}
