// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"github.com/azure/azure-dev/cli/azd/pkg/commands"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/spf13/cobra"
)

func infraCmd(rootOptions *commands.GlobalCommandOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "infra",
		Short: "Manage Azure resources",
	}

	cmd.AddCommand(output.AddOutputParam(
		infraCreateCmd(rootOptions),
		[]output.Format{output.JsonFormat, output.NoneFormat},
		output.NoneFormat,
	))
	cmd.AddCommand(infraDeleteCmd(rootOptions))
	return cmd
}
