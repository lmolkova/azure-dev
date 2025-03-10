// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

import * as vscode from 'vscode';
import * as tas from 'vscode-tas-client';
import { registerUIExtensionVariables, createAzExtOutputChannel, callWithTelemetryAndErrorHandling, IActionContext, createExperimentationService } from '@microsoft/vscode-azext-utils';
import ext from './ext';
import { registerCommands } from './commands/registerCommands';
import { DotEnvTaskProvider } from './tasks/dotEnvTaskProvider';
import { TelemetryId } from './telemetry/telemetryId';
import { scheduleSurveys } from './telemetry/surveyScheduler';
import { SurveyOne } from './telemetry/surveyOne';
import { ActivityStatisticsService } from './telemetry/activityStatisticsService';

type LoadStats = {
    // Both are the values returned by Date.now()==milliseconds since Unix epoch.
    loadStartTime: number,
    loadEndTime: number | undefined
};

export async function activateInternal(vscodeCtx: vscode.ExtensionContext, loadStats: LoadStats) {
    loadStats.loadEndTime = Date.now();

    function registerDisposable<T extends vscode.Disposable>(disposable: T): T {
        vscodeCtx.subscriptions.push(disposable);
        
        return disposable;
    }

    // The following is necessary for telemetry to work, so do this before callWithTelemetryAndErrorHandling()
    ext.context = vscodeCtx;
    ext.ignoreBundle = false;
    ext.outputChannel = registerDisposable(createAzExtOutputChannel('Azure Developer', 'az dev'));
    
    await callWithTelemetryAndErrorHandling(TelemetryId.Activation, async (activationCtx: IActionContext) => {
        activationCtx.errorHandling.rethrow = true;
        activationCtx.telemetry.properties.isActivationEvent = 'true';
        // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
        activationCtx.telemetry.measurements.mainFileLoadTime = (loadStats.loadEndTime! - loadStats.loadStartTime) / 1000.0; // Convert to seconds (vscode-azext-utils convention).

        // Now do all actual activation tasks.
        ext.userAgent = `${ext.azureDevExtensionNamespace}/v${vscodeCtx.extension.packageJSON.version}`;
        ext.experimentationSvc = await createExperimentationService(vscodeCtx, tas.TargetPopulation.Team);
        ext.activitySvc = new ActivityStatisticsService(vscodeCtx.globalState);
        registerUIExtensionVariables(ext);
        registerCommands();
        registerDisposable(vscode.tasks.registerTaskProvider('dotenv', new DotEnvTaskProvider()));
        scheduleSurveys(vscodeCtx.globalState, [SurveyOne]);
    });
}

export async function deactivateInternal(): Promise<void> {
    await callWithTelemetryAndErrorHandling(TelemetryId.Deactivation, (activationCtx: IActionContext) => {
        activationCtx.telemetry.properties.isActivationEvent = 'true';

        // We have no de-activation work to do today, but we might have some in future.
    });
}
