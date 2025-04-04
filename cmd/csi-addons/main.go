/*
Copyright 2022 The Ceph-CSI Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/csi-addons/kubernetes-csi-addons/internal/version"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	endpoint    = "unix:///tmp/csi-addons.sock"
	stagingPath = "/var/lib/kubelet/plugins/kubernetes.io/csi/"
)

// command contains the parsed arguments that were passed while running the
// executable.
type command struct {
	endpoint                      string
	stagingPath                   string
	operation                     string
	persistentVolume              string
	volumeGroupReplicationContent string
	drivername                    string
	secret                        string
	cidrs                         string
	clusterid                     string
	legacy                        bool
	volumeGroupName               string
	volumeGroupID                 string
	parameters                    string
	volumeIDs                     string
}

// cmd is the single instance of the command struct, used inside main().
var cmd = &command{}

func init() {
	var showVersion bool

	flag.StringVar(&cmd.endpoint, "endpoint", endpoint, "CSI-Addons endpoint")
	flag.StringVar(&cmd.stagingPath, "stagingpath", stagingPath, "staging path")
	flag.StringVar(&cmd.operation, "operation", "", "csi-addons operation")
	flag.StringVar(&cmd.persistentVolume, "persistentvolume", "", "name of the PersistentVolume")
	flag.StringVar(&cmd.volumeGroupReplicationContent, "volumegroupreplicationcontent", "", "name of the VolumeGroupReplicationContent")
	flag.StringVar(&cmd.drivername, "drivername", "", "name of the CSI driver")
	flag.StringVar(&cmd.secret, "secret", "", "kubernetes secret in the format `namespace/name`")
	flag.StringVar(&cmd.cidrs, "cidrs", "", "comma separated list of cidrs")
	flag.StringVar(&cmd.clusterid, "clusterid", "", "clusterID")
	flag.BoolVar(&cmd.legacy, "legacy", false, "use legacy format for old Kubernetes versions")
	flag.BoolVar(&showVersion, "version", false, "print Version details")
	flag.StringVar(&cmd.volumeGroupName, "volumegroupname", "", "name of the Volume Group to be created")
	flag.StringVar(&cmd.volumeGroupID, "volumegroupid", "", "ID of the volume group")
	flag.StringVar(&cmd.parameters, "parameters", "", "parameters in key=value format separated by commas(Eg:- k1=v1,k2=v2...)")
	flag.StringVar(&cmd.volumeIDs, "volumeids", "", "comma separated list of VolumeIds")

	// output to show when --help is passed
	flag.Usage = func() {
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintln(flag.CommandLine.Output(), "The following operations are supported:")
		for op := range operations {
			fmt.Fprintln(flag.CommandLine.Output(), " - "+op)
		}
		os.Exit(0)
	}

	flag.Parse()

	if showVersion {
		version.PrintVersion()
		os.Exit(0)
	}
}

func main() {
	op, found := operations[cmd.operation]
	if !found {
		log.Fatalf("ERROR: operation %q not found", cmd.operation)
	}

	op.Connect(cmd.endpoint)

	err := op.Init(cmd)
	if err == nil {
		defer func() {
			err = op.Close()
			if err != nil {
				log.Printf("failed to close connection: %v", err)
			}
		}()
	}

	if err != nil {
		err = fmt.Errorf("failed to initialize %q: %w", cmd.operation, err)
	} else {
		err = op.Execute()
		if err != nil {
			err = fmt.Errorf("failed to execute %q: %w", cmd.operation, err)
		}
	}

	if err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}

// getKubernetesClient returns a Clientset so that the Kubernetes API can be
// used. In case the Clientset can not be created, this function will panic as
// there will be no use of running the tool.
func getKubernetesClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	config.ContentType = runtime.ContentTypeProtobuf

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

// operations contain a list of all available operations. Each operation should
// be added by calling registerOperation().
var operations = make(map[string]operation)

// operation is the interface that all operations should implement. The
// Connect() and Close() functions can be inherited from the grpcClient struct.
type operation interface {
	Connect(endpoint string)
	Close() error

	Init(c *command) error
	Execute() error
}

// grpcClient provides standard Connect() and Close() functions that an
// operation needs to provide.
type grpcClient struct {
	Client *grpc.ClientConn
}

// Connect to the endpoint, or panic in case it fails.
func (g *grpcClient) Connect(endpoint string) {
	conn, err := grpc.NewClient(
		endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Sprintf("failed to connect to %q: %v", endpoint, err))
	}

	g.Client = conn
}

// Close the connected grpc.ClientConn.
func (g *grpcClient) Close() error {
	if g.Client == nil {
		return nil
	}

	return g.Client.Close()
}

// registerOperation adds a new operation struct to the operations map.
func registerOperation(name string, op operation) error {
	if _, ok := operations[name]; ok {
		return fmt.Errorf("operation %q is already registered", name)
	}

	operations[name] = op

	return nil
}
