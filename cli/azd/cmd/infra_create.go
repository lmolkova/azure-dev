package cmd

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/azure/azure-dev/cli/azd/pkg/azure"
	"github.com/azure/azure-dev/cli/azd/pkg/commands"
	"github.com/azure/azure-dev/cli/azd/pkg/environment"
	"github.com/azure/azure-dev/cli/azd/pkg/iac/bicep"
	"github.com/azure/azure-dev/cli/azd/pkg/infra"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/project"
	"github.com/azure/azure-dev/cli/azd/pkg/spin"
	"github.com/azure/azure-dev/cli/azd/pkg/tools"
	"github.com/drone/envsubst"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/theckman/yacspin"
	"go.uber.org/multierr"
)

type infraCreateAction struct {
	noProgress  bool
	rootOptions *commands.GlobalCommandOptions
}

func infraCreateCmd(rootOptions *commands.GlobalCommandOptions) *cobra.Command {
	cmd := commands.Build(
		&infraCreateAction{
			rootOptions: rootOptions,
		},
		rootOptions,
		"create",
		"Create Azure resources for an application",
		"",
	)

	cmd.Aliases = []string{"provision"}
	return cmd
}

func (ica *infraCreateAction) SetupFlags(persis, local *pflag.FlagSet) {
	local.BoolVar(&ica.noProgress, "no-progress", false, "Suppress progress information")
}

func (ica *infraCreateAction) Run(ctx context.Context, cmd *cobra.Command, args []string, azdCtx *environment.AzdContext) error {
	azCli := commands.GetAzCliFromContext(ctx)
	bicepCli := tools.NewBicepCli(azCli)
	askOne := makeAskOne(ica.rootOptions.NoPrompt)

	if err := ensureProject(azdCtx.ProjectPath()); err != nil {
		return err
	}

	if err := tools.EnsureInstalled(ctx, azCli, bicepCli); err != nil {
		return err
	}

	if err := ensureLoggedIn(ctx); err != nil {
		return fmt.Errorf("failed to ensure login: %w", err)
	}

	env, err := loadOrInitEnvironment(ctx, &ica.rootOptions.EnvironmentName, azdCtx, askOne)
	if err != nil {
		return fmt.Errorf("loading environment: %w", err)
	}

	_, err = project.LoadProjectConfig(azdCtx.ProjectPath(), &environment.Environment{})
	if err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	const rootModule = "main"

	// Copy the parameter template file to the environment working directory and do substitutions.
	parametersPath := azdCtx.BicepParametersTemplateFilePath(rootModule)
	parametersBytes, err := ioutil.ReadFile(parametersPath)
	if err != nil {
		return fmt.Errorf("reading parameter file template: %w", err)
	}
	replaced, err := envsubst.Eval(string(parametersBytes), func(name string) string {
		if val, has := env.Values[name]; has {
			return val
		}
		return os.Getenv(name)
	})
	if err != nil {
		return fmt.Errorf("substituting parameter file: %w", err)
	}
	err = ioutil.WriteFile(azdCtx.BicepParametersFilePath(ica.rootOptions.EnvironmentName, rootModule), []byte(replaced), 0644)
	if err != nil {
		return fmt.Errorf("writing parameter file: %w", err)
	}

	// Fetch the parameters from the template and ensure we have a value for each one, otherwise
	// prompt.
	bicepPath := azdCtx.BicepModulePath(rootModule)
	template, err := bicep.Compile(ctx, bicepCli, bicepPath)
	if err != nil {
		return err
	}

	// When creating a deployment, we need an azure location which is used to store the deployment metadata. This can be
	// any azure location and the choice doesn't impact what location individual resources in the deployment use. By default
	// we'll just use whatever value is being passed to the `location` parameter for bicep, and if that's not defined,
	// we'll prompt the user as to what location they want to use.
	//
	// TODO: The UX here could be improved. One problem is the concept of "the location used to store deployment metadata,
	// but not the resources" is sort of confusing and hard to clearly articulate.
	var location string

	if len(template.Parameters) > 0 {
		configuredParameters, err := azdCtx.BicepParameters(ica.rootOptions.EnvironmentName, rootModule)
		if err != nil {
			return fmt.Errorf("reading existing parameters: %w", err)
		}

		updatedParameters := false
		for parameter, value := range template.Parameters {
			// If this parameter has a default, then there is no need for us to configure it
			if _, hasDefault := value["defaultValue"]; hasDefault {
				continue
			}
			if _, has := configuredParameters[parameter]; !has {

				var val string
				if err := askOne(&survey.Input{
					Message: fmt.Sprintf("Please enter a value for the '%s' deployment parameter:", parameter),
				}, &val); err != nil {
					return fmt.Errorf("prompting for deployment parameter: %w", err)
				}

				configuredParameters[parameter] = val

				saveParameter := true
				if err := askOne(&survey.Confirm{
					Message: "Save the value in the environment for future use",
				}, &saveParameter); err != nil {
					return fmt.Errorf("prompting to save deployment parameter: %w", err)
				}

				if saveParameter {
					env.Values[parameter] = val
				}

				updatedParameters = true
			}

			if parameter == "location" {
				location = configuredParameters[parameter].(string)
			}
		}

		if updatedParameters {
			if err := azdCtx.WriteBicepParameters(ica.rootOptions.EnvironmentName, rootModule, configuredParameters); err != nil {
				return fmt.Errorf("saving deployment parameters: %w", err)
			}

			if err := env.Save(); err != nil {
				return fmt.Errorf("writing env file: %w", err)
			}
		}
	}

	for location == "" {
		// TODO: We will want to store this information somewhere (so we don't have to prompt the
		// user on every deployment if they don't have a `location` parameter in their bicep file.
		// When we store it, we should store it /per environment/ not as a property of the entire
		// project.
		selected, err := promptLocation(ctx, "Please select an Azure location to use to store deployment metadata:", askOne)
		if err != nil {
			return fmt.Errorf("prompting for deployment metadata region: %w", err)
		}

		location = selected
	}

	formatter, err := output.GetFormatter(cmd)
	if err != nil {
		return err
	}
	interactive := formatter.Kind() == output.NoneFormat

	// Do the creating. The call to `DeployToSubscription` blocks until the deployment completes,
	// which can take a bit, so we typically do some progress indication.
	// For interactive use (default case, using table formatter), we use a spinner.
	// With JSON formatter we emit progress information, unless --no-progress option was set.
	deploymentTarget := bicep.NewSubscriptionDeploymentTarget(azCli, location, env.GetSubscriptionId(), env.GetEnvName())

	type deployFuncResult struct {
		Result tools.AzCliDeployment
		Err    error
	}
	var res deployFuncResult

	deployAndReportProgress := func(showProgress func(string)) error {
		deployResChan := make(chan deployFuncResult)
		go func() {
			res, err := bicep.Deploy(ctx, deploymentTarget, bicepPath, azdCtx.BicepParametersFilePath(ica.rootOptions.EnvironmentName, "main"))
			deployResChan <- deployFuncResult{Result: res, Err: err}
			close(deployResChan)
		}()

		for {
			select {
			case deployRes := <-deployResChan:
				res = deployRes
				return deployRes.Err
			case <-time.After(10 * time.Second):
				if ica.noProgress {
					continue
				}
				if interactive {
					reportDeploymentStatusInteractive(ctx, azCli, env, showProgress)
				} else {
					reportDeploymentStatusJson(ctx, azCli, env, formatter, cmd)
				}
			}
		}
	}

	if interactive {
		deploymentSlug := azure.SubscriptionDeploymentRID(env.GetSubscriptionId(), env.GetEnvName())
		deploymentURL := withLinkFormat(
			"https://portal.azure.com/#blade/HubsExtension/DeploymentDetailsBlade/overview/id/%s\n\n",
			url.PathEscape(deploymentSlug))
		printWithStyling(
			"Provisioning Azure resources can take some time.\n\nYou can view detailed progress in the Azure Portal:\n%s",
			deploymentURL)
		//fmt.Fprintf(colorable.NewColorableStdout(), "Provisioning Azure resources can take some time.\n\nYou can view detailed progress in the Azure Portal:\n%s", deploymentURL)

		err = spin.RunWithUpdater("Creating Azure resources ", deployAndReportProgress,
			func(s *yacspin.Spinner, deploySuccess bool) {
				s.StopMessage("Created Azure resources\n")
			})
	} else {
		err = deployAndReportProgress(nil)
	}

	if err != nil {
		if formatter.Kind() == output.JsonFormat {
			deploy, deployErr := azCli.GetSubscriptionDeployment(ctx, env.GetSubscriptionId(), env.GetEnvName())
			if deployErr != nil {
				return fmt.Errorf("deployment failed and the deployment result is unavailable: %w", multierr.Combine(err, deployErr))
			}

			if fmtErr := formatter.Format(deploy, cmd.OutOrStdout(), nil); fmtErr != nil {
				return fmt.Errorf("deployment failed and the deployment result could not be displayed: %w", multierr.Combine(err, fmtErr))
			}
		}

		return fmt.Errorf("deployment failed: %w", err)
	}

	template.CanonicalizeDeploymentOutputs(&res.Result.Properties.Outputs)
	if err = saveEnvironmentValues(res.Result, env); err != nil {
		return err
	}

	if formatter.Kind() == output.JsonFormat {
		if err = formatter.Format(res.Result, cmd.OutOrStdout(), nil); err != nil {
			return fmt.Errorf("deployment result could not be displayed: %w", err)
		}
	}

	return nil
}

func reportDeploymentStatusInteractive(ctx context.Context, azCli tools.AzCli, env environment.Environment, showProgress func(string)) {
	resourceManager := infra.NewAzureResourceManager(azCli)

	operations, err := resourceManager.GetDeploymentResourceOperations(ctx, env.GetSubscriptionId(), env.GetEnvName())
	if err != nil {
		// Status display is best-effort activity.
		return
	}

	succeededCount := 0

	for _, resourceOperation := range *operations {
		if resourceOperation.Properties.ProvisioningState == "Succeeded" {
			succeededCount++
		}
	}

	status := fmt.Sprintf("Creating Azure resources (%d of ~%d completed) ", succeededCount, len(*operations))
	showProgress(status)
}

type progressReport struct {
	Timestamp  time.Time                      `json:"timestamp"`
	Operations []tools.AzCliResourceOperation `json:"operations"`
}

func reportDeploymentStatusJson(ctx context.Context, azCli tools.AzCli, env environment.Environment, formatter output.Formatter, cmd *cobra.Command) {
	resourceManager := infra.NewAzureResourceManager(azCli)

	ops, err := resourceManager.GetDeploymentResourceOperations(ctx, env.GetSubscriptionId(), env.GetEnvName())
	if err != nil || len(*ops) == 0 {
		// Status display is best-effort activity.
		return
	}

	report := progressReport{
		Timestamp:  time.Now(),
		Operations: *ops,
	}

	_ = formatter.Format(report, cmd.OutOrStdout(), nil)
}
