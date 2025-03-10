// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"log"

	"github.com/azure/azure-dev/cli/azd/pkg/commands"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/templates"
	"github.com/spf13/cobra"
)

func templatesCmd(rootOptions *commands.GlobalCommandOptions) *cobra.Command {
	root := &cobra.Command{
		Use:   "template",
		Short: "Manage templates",
	}

	root.AddCommand(output.AddOutputParam(
		templatesListCmd(rootOptions),
		[]output.Format{output.JsonFormat, output.TableFormat},
		output.TableFormat,
	))
	root.AddCommand(output.AddOutputParam(
		templatesShowCmd(rootOptions),
		[]output.Format{output.JsonFormat, output.TableFormat},
		output.TableFormat,
	))

	return root
}

func templatesListCmd(rootOptions *commands.GlobalCommandOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List templates",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			templateManager := templates.NewTemplateManager()
			templateList, err := templateManager.ListTemplates()

			if err != nil {
				return err
			}

			return formatTemplates(cmd, templateList...)
		},
	}
}

func templatesShowCmd(rootOptions *commands.GlobalCommandOptions) *cobra.Command {
	action := commands.ActionFunc(
		func(_ context.Context, cmd *cobra.Command, args []string, azdCtx *environment.AzdContext) error {
			templateName := args[0]
			templateManager := templates.NewTemplateManager()
			matchingTemplate, err := templateManager.GetTemplate(templateName)

			log.Printf("Template Name: %s\n", templateName)

			if err != nil {
				return err
			}

			return formatTemplates(cmd, matchingTemplate)
		},
	)
	cmd := commands.Build(
		action,
		rootOptions,
		"show <template>",
		"Show the template details",
		"",
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func formatTemplates(cmd *cobra.Command, templates ...templates.Template) error {
	formatter, err := output.GetFormatter(cmd)
	if err != nil {
		return err
	}

	if formatter.Kind() == output.TableFormat {
		columns := []output.Column{
			{
				Heading:       "Name",
				ValueTemplate: "{{.Name}}",
			},
			{
				Heading:       "Description",
				ValueTemplate: "{{.Description}}",
			},
		}

		err = formatter.Format(templates, cmd.OutOrStdout(), output.TableFormatterOptions{
			Columns: columns,
		})
	} else {
		err = formatter.Format(templates, cmd.OutOrStdout(), nil)
	}

	if err != nil {
		return err
	}

	return nil
}
