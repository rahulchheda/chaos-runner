package main

import (
	"time"

	"fmt"
	"github.com/litmuschaos/chaos-runner/pkg/utils"
	"github.com/litmuschaos/chaos-runner/pkg/utils/analytics"
	"k8s.io/klog"
	"os"
	"os/signal"
	"syscall"
)

func handleSig(ch chan os.Signal, sigCatcher *bool) {
	sig := <-ch
	*sigCatcher = true
	fmt.Printf("Sig Signal Caught: %v \n Value of SIGCATCHER: %v \n", sig, sigCatcher)
	return
}

var sigCatcher = false

func main() {
	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go handleSig(sig, &sigCatcher)
	engineDetails := utils.EngineDetails{}
	clients := utils.ClientSets{}

	// Getting kubeConfig and Generate ClientSets
	if err := clients.GenerateClientSetFromKubeConfig(); err != nil {
		klog.V(0).Infof("Unable to create ClientSets, error: %v", err)
		return
	}

	// Fetching all the ENV's needed
	utils.GetOsEnv(&engineDetails)
	klog.V(0).Infoln("Experiments List: ", engineDetails.Experiments, " ", "Engine Name: ", engineDetails.Name, " ", "appLabels : ", engineDetails.AppLabel, " ", "appNamespace: ", engineDetails.AppNamespace, " ", "appKind: ", engineDetails.AppKind, " ", "Service Account Name: ", engineDetails.SvcAccount)

	// Steps for each Experiment
	for i := range engineDetails.Experiments {

		// Sending event to GA instance
		if engineDetails.ClientUUID != "" {
			analytics.TriggerAnalytics(engineDetails.Experiments[i], engineDetails.ClientUUID)
		}
		experiment := utils.NewExperimentDetails()

		experiment.SetValueFromChaosEngine(&engineDetails, i)

		if err := engineDetails.SetValueFromChaosRunner(clients); err != nil {
			klog.V(0).Infof("Unable to set values from ChaosRunner, error: %v", err)
			break
		}

		if err := experiment.SetValueFromChaosExperiment(clients, &engineDetails); err != nil {
			klog.V(0).Infof("Unable to set values from chaosExperiments %v", err)
			break
		}
		if err := experiment.SetENV(engineDetails, clients); err != nil {
			klog.V(0).Infof("Unable to patch ENV, due to error: %v", err)
			break
		}
		experimentStatus := utils.ExperimentStatus{}
		experimentStatus.InitialExperimentStatus(experiment)
		if err := experimentStatus.InitialPatchEngine(engineDetails, clients); err != nil {
			klog.V(0).Infof("Unable to set Initial Status in ChaosEngine, due to error: %v", err)
		}

		klog.V(0).Infof("Preparing to run Chaos Experiment: %v", experiment.Name)

		if err := experiment.HandleChaosExperimentExistence(engineDetails, clients); err != nil {
			klog.V(0).Infof("Unable to get ChaosExperiment Name: %v, in namespace: %v, due to error: %v", experiment.Name, experiment.Namespace, err)
			break
		}

		if err := experiment.PatchResources(engineDetails, clients); err != nil {
			klog.V(0).Infof("Unable to patch Chaos Resources required for Chaos Experiment: %v, due to error: %v", experiment.Name, err)
		}

		// Creation of PodTemplateSpec, and Final Job
		if err := utils.BuildingAndLaunchJob(experiment, clients); err != nil {
			klog.V(0).Infof("Unable to construct chaos experiment job due to: %v", err)
			break
		}

		time.Sleep(5 * time.Second)

		klog.V(0).Infof("Started Chaos Experiment Name: %v, with Job Name: %v", experiment.Name, experiment.JobName)
		// Watching the Job till Completion
		if err := engineDetails.WatchJobForCompletion(experiment, clients); err != nil {
			klog.V(0).Infof("Unable to Watch the Job, error: %v", err)
			break
		}

		// Will Update the chaosEngine Status
		if err := engineDetails.UpdateEngineWithResult(experiment, clients); err != nil {
			klog.V(0).Infof("Unable to Update ChaosEngine Status due to: %v", err)
		}

		// Delete / retain the Job, using the jobCleanUpPolicy
		if err := engineDetails.DeleteJobAccordingToJobCleanUpPolicy(experiment, clients); err != nil {
			klog.V(0).Infof("Unable to Delete chaosExperiment Job due to: %v", err)
		}
	}
}
