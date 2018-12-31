package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	pvsClient := clientset.CoreV1().PersistentVolumes()
	pvcsClient := clientset.CoreV1().PersistentVolumeClaims(corev1.NamespaceDefault)
	deploymentsClient := clientset.AppsV1().Deployments(corev1.NamespaceDefault)
	servicesClient := clientset.CoreV1().Services(corev1.NamespaceDefault)

	pv := &corev1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "task-pv-volume-client",
			Labels: map[string]string{
				"type": "local-client",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany, corev1.ReadWriteOnce},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/tmp/data",
					Type: hostpathtypeptr(corev1.HostPathDirectoryOrCreate),
				},
			},
		},
	}

	fmt.Println("Creating Persistent Volume...")
	resultPv, err := pvsClient.Create(pv)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created Persistent Volume - Name: %q, UID: %q\n", resultPv.GetObjectMeta().GetName(), resultPv.GetObjectMeta().GetUID())

	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolumeClaim",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "task-pv-claim-client",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("3Gi"),
				},
			},
		},
	}
	waitForEnter()
	fmt.Println("Creating Persistent Volume Claim...")
	resultPvc, err := pvcsClient.Create(pvc)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created PVC - Name: %q, UID: %q\n", resultPvc.GetObjectMeta().GetName(), resultPvc.GetObjectMeta().GetUID())

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "booklistkube-client",
			Namespace: "default",
			Labels: map[string]string{
				"app": "booklistkube-client",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32ptr(3),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "booklistkube-client",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "booklistkube-client",
					Labels: map[string]string{
						"app": "booklistkube-client",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "booklistkube-client",
							Image:           "kfoozminus/booklist:alpine",
							ImagePullPolicy: corev1.PullIfNotPresent,
							//Command:         []string{"/bin/sh", "-c", "echo hello; sleep 36000"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "task-pv-storage-client",
									MountPath: "/etc/pvc",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "exposedc",
									ContainerPort: 4321,
									Protocol:      corev1.ProtocolTCP,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyAlways,
					Volumes: []corev1.Volume{
						{
							Name: "task-pv-storage-client",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "task-pv-claim-client",
								},
							},
						},
					},
				},
			},
		},
	}
	waitForEnter()
	fmt.Println("Creating Deployment...")
	resultDeployment, err := deploymentsClient.Create(deployment)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created Deployment - Name: %q, UID: %q\n", resultDeployment.GetObjectMeta().GetName(), resultDeployment.GetObjectMeta().GetUID())

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "booklistkube-client",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "booklistkube-client",
			},
			Ports: []corev1.ServicePort{
				{
					Name: "exposeds",
					Port: 1234,
					//TargetPort: intstr.IntOrString{StrVal: "exposedc"},
					TargetPort: intstr.IntOrString{IntVal: 4321},
				},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}
	waitForEnter()
	fmt.Println("Creating Service...")
	//time.Sleep(60 * time.Second)
	resultService, err := servicesClient.Create(service)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Created Service - Name: %q, UID: %q\n", resultService.GetObjectMeta().GetName(), resultService.GetObjectMeta().GetUID())

	waitForEnter()
	fmt.Println("Updating Deployment...")
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		resultDeployment, getErr := deploymentsClient.Get("booklistkube-client", metav1.GetOption)
	})

}

func waitForEnter() {
	fmt.Println("..... Press Enter to Continue .....")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

func int32ptr(i int32) *int32                                               { return &i }
func hostpathtypeptr(hostpathtype corev1.HostPathType) *corev1.HostPathType { return &hostpathtype }
