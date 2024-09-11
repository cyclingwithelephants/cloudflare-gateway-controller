package k8s

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	DeploymentConfigFilePath     = "/etc/cloudflared/config/config.yaml"
	DeploymentCredentialFilePath = "/etc/cloudflared/creds/creds.json"
)

func BuildTunnelDeployment(deploymentName string, namespace string, tunnelId string) *appsv1.Deployment {
	replicas := int32(1)
	labels := map[string]string{
		"app.kubernetes.io/deploymentName": deploymentName,
	}
	maxSurge := intstr.FromString("1")
	maxUnavailable := intstr.FromString("0")
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			// by ensuring the new tunnel is up before the previous one is taken down
			// we reduce downtime associated with new cloudflared configurations from configMap updates
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &maxUnavailable,
					MaxSurge:       &maxSurge,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image: "cloudflare/cloudflared:latest",
						Name:  "cloudflared",
						Args: []string{
							"tunnel",
							"--protocol", "auto",
							"--config", DeploymentConfigFilePath,
							"--metrics", "0.0.0.0:2000",
							"run",
							"test",
						},
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/ready",
									Port: intstr.IntOrString{IntVal: 2000},
								},
							},
							FailureThreshold:    1,
							InitialDelaySeconds: 10,
							PeriodSeconds:       10,
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "metrics",
								ContainerPort: 2000,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "config",
							MountPath: "/etc/cloudflared/config",
							ReadOnly:  true,
						}, {
							Name:      "creds",
							MountPath: "/etc/cloudflared/creds",
							ReadOnly:  true,
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{"memory": resource.MustParse("30Mi"), "cpu": resource.MustParse("10m")},
							Limits:   corev1.ResourceList{"memory": resource.MustParse("256Mi"), "cpu": resource.MustParse("500m")},
						},
					}},
					Volumes: []corev1.Volume{{
						Name: "creds",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: secretName(deploymentName)},
						},
					}, {
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: ConfigMapName(deploymentName)},
								Items: []corev1.KeyToPath{{
									Key:  "config.yaml",
									Path: "config.yaml",
								}},
							},
						},
					}},
				},
			},
		},
	}
}
