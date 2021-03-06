package utils

import (
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/litmuschaos/kube-helper/kubernetes/container"
	"github.com/litmuschaos/kube-helper/kubernetes/job"
	jobspec "github.com/litmuschaos/kube-helper/kubernetes/jobspec"
	"github.com/litmuschaos/kube-helper/kubernetes/podtemplatespec"
)

// PodTemplateSpec is struct for creating the *core1.PodTemplateSpec
type PodTemplateSpec struct {
	Object *corev1.PodTemplateSpec
}

// Builder struct for getting the error as well with the template
type Builder struct {
	podtemplatespec *PodTemplateSpec
	errs            []error
}

// BuildContainerSpec builds a Container with following properties
func buildContainerSpec(experiment *ExperimentDetails, envVar []corev1.EnvVar) (*container.Builder, error) {
	containerSpec := container.NewBuilder().
		WithName(experiment.JobName).
		WithImage(experiment.ExpImage).
		WithCommandNew([]string{"/bin/bash"}).
		WithArgumentsNew(experiment.ExpArgs).
		WithImagePullPolicy("Always").
		WithEnvsNew(envVar)

	if experiment.VolumeOpts.VolumeMounts != nil {
		containerSpec.WithVolumeMountsNew(experiment.VolumeOpts.VolumeMounts)
	}

	_, err := containerSpec.Build()

	if err != nil {
		return nil, err
	}

	return containerSpec, err

}

func getEnvFromMap(env map[string]string) []corev1.EnvVar {
	var envVar []corev1.EnvVar
	for k, v := range env {
		var perEnv corev1.EnvVar
		perEnv.Name = k
		perEnv.Value = v
		envVar = append(envVar, perEnv)
	}
	return envVar
}

// BuildingAndLaunchJob builds Job, and then launch it.
func BuildingAndLaunchJob(experiment *ExperimentDetails, clients ClientSets) error {
	experiment.VolumeOpts.VolumeOperations(experiment.ConfigMaps, experiment.Secrets)

	envVar := getEnvFromMap(experiment.Env)
	//Build Container to add in the Pod
	containerForPod, err := buildContainerSpec(experiment, envVar)
	if err != nil {
		return errors.Wrapf(err, "Unable to build Container for Chaos Experiment, due to error: %v", err)
	}
	// Will build a PodSpecTemplate
	pod, err := buildPodTemplateSpec(experiment, containerForPod)
	if err != nil {

		return errors.Wrapf(err, "Unable to build PodTemplateSpec for Chaos Experiment, due to error: %v", err)
	}
	// Build JobSpec Template
	jobspec, err := buildJobSpec(pod)
	if err != nil {
		return err
	}
	//Build Job
	job, err := experiment.buildJob(pod, jobspec)
	if err != nil {
		return errors.Wrapf(err, "Unable to Build ChaosExperiment Job, due to error: %v", err)
	}
	// Creating the Job
	if err = experiment.launchJob(job, clients); err != nil {
		return errors.Wrapf(err, "Unable to launch ChaosExperiment Job, due to error: %v", err)
	}
	return nil
}

// launchJob spawn a kubernetes Job using the job Object recieved.
func (experiment *ExperimentDetails) launchJob(job *batchv1.Job, clients ClientSets) error {
	_, err := clients.KubeClient.BatchV1().Jobs(experiment.Namespace).Create(job)
	if err != nil {
		return err
	}
	return nil
}

// BuildPodTemplateSpec return a PodTempplateSpec
func buildPodTemplateSpec(experiment *ExperimentDetails, containerForPod *container.Builder) (*podtemplatespec.Builder, error) {
	podtemplate := podtemplatespec.NewBuilder().
		WithName(experiment.JobName).
		WithNamespace(experiment.Namespace).
		WithLabels(experiment.ExpLabels).
		WithServiceAccountName(experiment.SvcAccount).
		WithRestartPolicy(corev1.RestartPolicyOnFailure).
		WithVolumeBuilders(experiment.VolumeOpts.VolumeBuilders).
		WithContainerBuildersNew(containerForPod)

	if _, err := podtemplate.Build(); err != nil {
		return nil, err
	}
	return podtemplate, nil
}

// BuildJobSpec returns a JobSpec
func buildJobSpec(pod *podtemplatespec.Builder) (*jobspec.Builder, error) {
	jobSpecObj := jobspec.NewBuilder().
		WithPodTemplateSpecBuilder(pod)
	_, err := jobSpecObj.Build()
	if err != nil {
		return nil, err
	}
	return jobSpecObj, nil
}

// BuildJob will build the JobObject for creation
func (experiment *ExperimentDetails) buildJob(pod *podtemplatespec.Builder, jobspec *jobspec.Builder) (*batchv1.Job, error) {
	//restartPolicy := corev1.RestartPolicyOnFailure
	jobObj, err := job.NewBuilder().
		WithJobSpecBuilder(jobspec).
		WithName(experiment.JobName).
		WithNamespace(experiment.Namespace).
		WithLabels(experiment.ExpLabels).
		Build()
	if err != nil {
		return jobObj, err
	}
	return jobObj, nil
}
