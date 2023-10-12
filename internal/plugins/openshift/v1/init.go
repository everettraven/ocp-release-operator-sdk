// Copyright 2021 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/spf13/afero"
	"sigs.k8s.io/kubebuilder/v3/pkg/config"
	"sigs.k8s.io/kubebuilder/v3/pkg/machinery"
)

const (
	// The current OCP release version.
	ocpProductVersion = "4.14"
	// The currently used version of ubi8/ubi-minimal images.
	ubiMinimalVersion = "8.8"
)

type initSubcommand struct {
	config config.Config
}

func (s *initSubcommand) InjectConfig(c config.Config) error {
	s.config = c
	return nil
}

// Scaffold updates a newly initialized project with OpenShift-specific configuration.
func (s *initSubcommand) Scaffold(fs machinery.Filesystem) error {
	if err := replaceImages(fs); err != nil {
		return err
	}

	// Update the plugin config section with this plugin's configuration.
	if err := s.config.EncodePluginConfig(pluginKey, Config{}); err != nil && !errors.As(err, &config.UnsupportedFieldError{}) {
		return fmt.Errorf("error writing plugin config for %s: %v", pluginKey, err)
	}

	return nil
}

type substitution struct {
	fromTagRE *regexp.Regexp
	toTag     string
}

// imageSubstitutions is a map of paths to image substitutions.
var imageSubstitutions = map[string][]substitution{
	filepath.Join("config", "default", "manager_auth_proxy_patch.yaml"): {
		{
			regexp.MustCompile(`gcr.io/kubebuilder/kube-rbac-proxy:[^ \n]+`),
			"registry.redhat.io/openshift4/ose-kube-rbac-proxy:v" + ocpProductVersion,
		},
	},
	filepath.Join("Dockerfile"): {
		// Ansible
		{
			regexp.MustCompile(`quay.io/operator-framework/ansible-operator:[^ \n]+`),
			"registry.redhat.io/openshift4/ose-ansible-operator:v" + ocpProductVersion,
		},
		// Helm
		{
			regexp.MustCompile(`quay.io/operator-framework/helm-operator:[^ \n]+`),
			"registry.redhat.io/openshift4/ose-helm-operator:v" + ocpProductVersion,
		},
		// Go
		{
			regexp.MustCompile(`gcr.io/distroless/static:[^ \n]+`),
			"registry.access.redhat.com/ubi8/ubi-minimal:" + ubiMinimalVersion,
		},
		// Go - for https://access.redhat.com/security/cve/CVE-2023-44487 &&
		// https://access.redhat.com/security/cve/CVE-2023-39325 .
		// This will ensure all Go projects have their Dockerfile updated to use
		// Go 1.20+ as the builder image. This should be removed when the default
		// scaffolds are updated to be Go 1.20+
		{
			regexp.MustCompile(`golang:[^ \n]+`),
			"golang:1.20",
		},
		// Hybrid Helm
		{
			regexp.MustCompile(`registry.access.redhat.com/ubi8/ubi-micro:[^ \n]+`),
			"registry.access.redhat.com/ubi8/ubi-micro:" + ubiMinimalVersion,
		},
	},

	filepath.Join("go.mod"): {
		// Go - for https://access.redhat.com/security/cve/CVE-2023-44487 &&
		// https://access.redhat.com/security/cve/CVE-2023-39325 .
		// This will ensure all Go projects have their go.mod updated to use
		// golang.org/x/net v0.17.0. This should be removed when the default
		// scaffolds are updated to use this by default
		{
			regexp.MustCompile(`golang.org/x/net [^ \n]+`),
			"golang.org/x/net v0.17.0",
		},
		// Go - for https://access.redhat.com/security/cve/CVE-2023-44487 &&
		// https://access.redhat.com/security/cve/CVE-2023-39325 .
		// This will ensure all Go projects have their go.mod updated to use
		// go 1.20+. This should be removed when the default
		// scaffolds are updated to use this by default
		{
			regexp.MustCompile(`go 1.[^2\n]+`),
			"go 1.20",
		},
	},
}

// replaceImages replaces upstream images with their downstream (OpenShift) equivalents.
func replaceImages(fs machinery.Filesystem) error {

	for filePath, substitutions := range imageSubstitutions {
		b, err := afero.ReadFile(fs.FS, filePath)
		if err != nil {
			return fmt.Errorf("error reading file for substitution: %v", err)
		}
		info, err := fs.FS.Stat(filePath)
		if err != nil {
			return fmt.Errorf("error reading file info for substitution: %v", err)
		}
		for _, subst := range substitutions {
			b = subst.fromTagRE.ReplaceAll(b, []byte(subst.toTag))
		}
		if err = afero.WriteFile(fs.FS, filePath, b, info.Mode()); err != nil {
			return err
		}
	}

	return nil
}
