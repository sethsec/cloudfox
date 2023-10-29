package cli

import (
	"log"

	"github.com/BishopFox/cloudfox/azure"
	"github.com/spf13/cobra"
)

var (
	AzTenantID        string
	AzSubscription    string
	AzRGName          string
	AzOutputFormat    string
	AzOutputDirectory string
	AzVerbosity       int
	AzWrapTable       bool
	AzMergedTable     bool

	AzResourceIDs      []string

	AzCommands = &cobra.Command{
		Use:     "azure",
		Aliases: []string{"az"},
		Long:    `See "Available Commands" for Azure Modules below`,
		Short:   "See \"Available Commands\" for Azure Modules below",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	AzWhoamiListRGsAlso bool
	AzWhoamiCommand     = &cobra.Command{
		Use:     "whoami",
		Aliases: []string{},
		Short:   "Display available Azure CLI sessions",
		Long: `
Display Available Azure CLI Sessions:
./cloudfox az whoami`,
		Run: func(cmd *cobra.Command, args []string) {
			err := azure.AzWhoamiCommand(AzOutputDirectory, cmd.Root().Version, AzWrapTable, AzVerbosity, AzWhoamiListRGsAlso)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
	AzInventoryCommand = &cobra.Command{
		Use:     "inventory",
		Aliases: []string{"inv"},
		Short:   "Display an inventory table of all resources per location",
		Long: `
Enumerate inventory for a specific tenant:
./cloudfox az inventory --tenant TENANT_ID

Enumerate inventory for a specific subscription:
./cloudfox az inventory --subscription SUBSCRIPTION_ID
`,
		Run: func(cmd *cobra.Command, args []string) {
			err := azure.AzInventoryCommand(AzTenantID, AzSubscription, AzOutputDirectory, cmd.Root().Version, AzVerbosity, AzWrapTable, AzMergedTable)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
	AzRBACCommand = &cobra.Command{
		Use:     "rbac",
		Aliases: []string{},
		Short:   "Display role assignemts for Azure principals",
		Long: `
Enumerate role assignments for a specific tenant:
./cloudfox az rbac --tenant TENANT_ID

Enumerate role assignments for a specific subscription:
./cloudfox az rbac --subscription SUBSCRIPTION_ID
`,
		Run: func(cmd *cobra.Command, args []string) {

			err := azure.AzRBACCommand(AzTenantID, AzSubscription, AzOutputFormat, AzOutputDirectory, cmd.Root().Version, AzVerbosity, AzWrapTable, AzMergedTable)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
	AzVMsCommand = &cobra.Command{
		Use:     "vms",
		Aliases: []string{"vms", "virtualmachines"},
		Short:   "Enumerates Azure Compute virtual machines",
		Long: `
Enumerate VMs for a specific tenant:
./cloudfox az vms --tenant TENANT_ID

Enumerate VMs for a specific subscription:
./cloudfox az vms --subscription SUBSCRIPTION_ID`,
		Run: func(cmd *cobra.Command, args []string) {
			err := azure.AzVMsCommand(AzTenantID, AzSubscription, AzOutputFormat, AzOutputDirectory, cmd.Root().Version, AzVerbosity, AzWrapTable, AzMergedTable)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
	AzStorageCommand = &cobra.Command{
		Use:     "storage",
		Aliases: []string{},
		Short:   "Enumerates azure storage accounts",
		Long: `
Enumerate storage accounts for a specific tenant:
./cloudfox az storage --tenant TENANT_ID

Enumerate storage accounts for a specific subscription:
./cloudfox az storage --subscription SUBSCRIPTION_ID
`,
		Run: func(cmd *cobra.Command, args []string) {
			err := azure.AzStorageCommand(AzTenantID, AzSubscription, AzOutputFormat, AzOutputDirectory, cmd.Root().Version, AzVerbosity, AzWrapTable, AzMergedTable)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
/*
	AzNSGCommand = &cobra.Command{
		Use:     "nsg",
		Aliases: []string{},
		Short:   "Enumerates azure Network Securiy Groups (NSG)",
		Long: `
Enumerate Network Security Groups rules for a specific tenant:
./cloudfox az nsg --tenant TENANT_ID

Enumerate Network Security Groups rules for a specific subscription:
./cloudfox az nsg --subscription SUBSCRIPTION_ID

Enumerate rules for a specific Network Security Group:
./cloudfox az nsg --nsg NSG_ID
`,
		Run: func(cmd *cobra.Command, args []string) {
			err := azure.AzNSGCommand(AzTenantID, AzSubscription, AzOutputFormat, AzOutputDirectory, cmd.Root().Version, AzVerbosity, AzWrapTable, AzMergedTable)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
*/
	AzNSGLinksCommand = &cobra.Command{
		Use:     "nsg-links",
		Aliases: []string{},
		Short:   "Enumerates azure Network Securiy Groups links",
		Long: `
Enumerate Network Security Groups links for a specific tenant:
./cloudfox az nsg-links --tenant TENANT_ID

Enumerate Network Security Groups links for a specific subscription:
./cloudfox az nsg-links --subscription SUBSCRIPTION_ID

Enumerate links for a specific Network Security Group:
./cloudfox az nsg-links --nsg NSG_ID
`,
		Run: func(cmd *cobra.Command, args []string) {
			err := azure.AzNSGLinksCommand(AzTenantID, AzSubscription, AzResourceIDs, AzOutputFormat, AzOutputDirectory, cmd.Root().Version, AzVerbosity, AzWrapTable, AzMergedTable)
			if err != nil {
				log.Fatal(err)
			}
		},
	}
)

func init() {

	AzWhoamiCommand.Flags().BoolVarP(&AzWhoamiListRGsAlso, "list-rgs", "l", false, "Drill down to the resource group level")

	// Global flags
	AzCommands.PersistentFlags().StringVarP(&AzOutputFormat, "output", "o", "all", "[\"table\" | \"csv\" | \"all\" ]")
	AzCommands.PersistentFlags().StringVar(&AzOutputDirectory, "outdir", defaultOutputDir, "Output Directory ")
	AzCommands.PersistentFlags().IntVarP(&AzVerbosity, "verbosity", "v", 2, "1 = Print control messages only\n2 = Print control messages, module output\n3 = Print control messages, module output, and loot file output\n")
	AzCommands.PersistentFlags().StringVarP(&AzTenantID, "tenant", "t", "", "Tenant name")
	AzCommands.PersistentFlags().StringVarP(&AzSubscription, "subscription", "s", "", "Subscription ID or Name")
	AzCommands.PersistentFlags().StringVarP(&AzRGName, "resource-group", "g", "", "Resource Group name")
	AzCommands.PersistentFlags().StringSliceVarP(&AzResourceIDs, "resource-id", "r", []string{}, "Resource ID (/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/{resourceProviderNamespace}/{resourceType}/{resourceName})")
	AzCommands.PersistentFlags().BoolVarP(&AzWrapTable, "wrap", "w", false, "Wrap table to fit in terminal (complicates grepping)")
	AzCommands.PersistentFlags().BoolVarP(&AzMergedTable, "merged-table", "m", false, "Writes a single table for all subscriptions in the tenant. Default writes a table per subscription.")

	AzCommands.AddCommand(
		AzWhoamiCommand,
		AzRBACCommand,
		AzVMsCommand,
		AzStorageCommand,
//		AzNSGCommand,
		AzNSGLinksCommand,
		AzInventoryCommand)

}
