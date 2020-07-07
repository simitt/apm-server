// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package idxmgmt

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/pkg/errors"

	libidxmgmt "github.com/elastic/beats/v7/libbeat/idxmgmt"
	libilm "github.com/elastic/beats/v7/libbeat/idxmgmt/ilm"
	"github.com/elastic/beats/v7/libbeat/template"

	"github.com/elastic/apm-server/idxmgmt/common"
	"github.com/elastic/apm-server/idxmgmt/ilm"
	"github.com/elastic/apm-server/utility"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

const (
	msgErrIlmDisabledES          = "automatically disabled ILM as not supported by configured Elasticsearch"
	msgIlmDisabledES             = "Automatically disabled ILM as configured Elasticsearch not eligible for auto enabling."
	msgIlmDisabledCfg            = "Automatically disabled ILM as custom index settings configured."
	msgIdxCfgIgnored             = "Custom index configuration ignored when ILM is enabled."
	msgIlmSetupDisabled          = "Manage ILM setup is disabled. "
	msgIlmSetupOverwriteDisabled = "Overwrite ILM setup is disabled. "
	msgTemplateSetupDisabled     = "Template loading is disabled. "

	patternSuffix = "*"
)

type manager struct {
	supporter     *supporter
	clientHandler libidxmgmt.ClientHandler
	assets        libidxmgmt.Asseter
}

func (m *manager) VerifySetup(loadTemplate, loadILM libidxmgmt.LoadMode) (bool, string) {
	templateFeature := m.templateFeature(loadTemplate)
	ilmFeature := m.ilmFeature(loadILM)

	if err := ilmFeature.error(); err != nil {
		return false, err.Error()
	}

	var warn string
	if !templateFeature.load {
		warn += msgTemplateSetupDisabled
	}
	if ilmWarn := ilmFeature.warning(); ilmWarn != "" {
		warn += ilmWarn
	}
	return warn == "", warn
}

func (m *manager) Setup(loadTemplate, loadILM libidxmgmt.LoadMode) error {
	// prepare template and ilm handlers, check if ILM is supported, fall back to ordinary index handling otherwise
	ilmFeature := m.ilmFeature(loadILM)
	if info := ilmFeature.information(); info != "" {
		m.supporter.log.Info(info)
	}
	if warn := ilmFeature.warning(); warn != "" {
		m.supporter.log.Warn(warn)
	}
	if err := ilmFeature.error(); err != nil {
		m.supporter.log.Error(err)
	}
	templateFeature := m.templateFeature(loadTemplate)
	m.supporter.templateConfig.Enabled = templateFeature.enabled
	m.supporter.templateConfig.Overwrite = templateFeature.overwrite

	fmt.Println(m.supporter.ilmConfig.Setup.Kind)
	fmt.Println(fmt.Sprintf("index templates supported:  %v", m.clientHandler.SupportsIndexTemplates()))
	if m.supporter.ilmConfig.Setup.Kind == template.KindLegacy ||
		!m.clientHandler.SupportsIndexTemplates() {

		return m.setupLegacy(templateFeature, ilmFeature)
	}
	if ilmFeature.enabled {
		return m.setupManaged(templateFeature, ilmFeature)
	}
	return m.setupUnmanaged(templateFeature, ilmFeature)
}

//TODO(simitt): add data stream handling!
func (m *manager) setupManaged(templateFeature, ilmFeature feature) error {
	if !templateFeature.load && !ilmFeature.load {
		m.supporter.log.Infof("ILM is enabled but setup is disabled. For full setup " +
			"ensure `apm-server.ilm.setup.enabled` and `setup.template.enabled` are set to `true`. ")
		return nil
	}

	// first disable generic apm template that would lead to conflicts with more specific templates

	// load generic component template
	baseComponentName := m.supporter.templateConfig.Name
	if baseComponentName == "" {
		baseComponentName = common.APMPrefix
	}
	if err := m.loadGenericComponentTemplate(templateFeature, baseComponentName); err != nil {
		return err
	}
	// load index template for default apm index - with disabled index pattern
	composedOf := []string{baseComponentName}
	if err := m.loadIndexTemplate(ilmFeature, baseComponentName, disabledPattern(), composedOf, false); err != nil {
		return err
	}

	// load index template for onboarding and sourcemap index
	for _, t := range []string{"sourcemap", "onboarding"} {
		name := fmt.Sprintf("%s-%s", common.APMPrefix, t)
		composedOf := []string{baseComponentName}
		m.loadIndexTemplate(ilmFeature, name, name+patternSuffix, composedOf, false)
	}

	// load event specific templates, policies and aliases
	var policiesLoaded []string
	var err error
	for _, ilmSupporter := range m.supporter.ilmSupporters {
		// load event type policies
		if policiesLoaded, err = m.loadPolicy(ilmFeature, ilmSupporter, policiesLoaded); err != nil {
			return err
		}
		name := ilmSupporter.Alias().Name

		withDataStreams := m.clientHandler.SupportsDataStream()
		// load component template per event type (with ILM settings)
		m.loadEventComponentTemplate(ilmFeature, name, ilmSupporter.Policy().Name, withDataStreams)

		// load index template per event type
		composedOf := []string{baseComponentName, name}
		m.loadIndexTemplate(ilmFeature, name, name+patternSuffix, composedOf, withDataStreams)

		// only load regular ILM write alias if data streams are not supported
		if !withDataStreams {
			// load ilm write aliases AFTER template creation
			if err = m.loadLegacyAlias(ilmFeature, ilmSupporter); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *manager) setupUnmanaged(templateFeature, ilmFeature feature) error {
	if !templateFeature.load && !ilmFeature.load {
		m.supporter.log.Infof("ILM is enabled but setup is disabled. For full setup " +
			"ensure `apm-server.ilm.setup.enabled` and `setup.template.enabled` are set to `true`. ")
		return nil
	}

	// first disable existing index templates that would lead to conflicts with generic apm template
	// then load generic apm template

	// load index template for onboarding and sourcemap index with non-matching index pattern
	for _, t := range []string{"sourcemap", "onboarding"} {
		name := fmt.Sprintf("%s-%s", common.APMPrefix, t)
		m.loadIndexTemplate(ilmFeature, name, disabledPattern(), []string{}, false)
	}
	// load event specific index templates with non-matching index pattern
	for _, ilmSupporter := range m.supporter.ilmSupporters {
		name := ilmSupporter.Alias().Name
		m.loadIndexTemplate(ilmFeature, name, disabledPattern(), []string{}, false)
	}

	// load generic component template
	baseComponentName := m.supporter.templateConfig.Name
	if baseComponentName == "" {
		baseComponentName = common.APMPrefix
	}
	if err := m.loadGenericComponentTemplate(templateFeature, baseComponentName); err != nil {
		return err
	}
	// load index template for default apm index
	pattern := m.supporter.templateConfig.Pattern
	if pattern == "" {
		pattern = baseComponentName + patternSuffix
	}
	composedOf := []string{baseComponentName}
	if err := m.loadIndexTemplate(ilmFeature, baseComponentName, pattern, composedOf, false); err != nil {
		return err
	}

	return nil
}

func (m *manager) setupLegacy(templateFeature, ilmFeature feature) error {
	// load general apm template
	if templateFeature.load {
		// if not customized, set the APM template name and pattern to the
		// default index prefix for managed and unmanaged indices;
		// in case the index/rollover_alias names were customized
		templateCfg := m.supporter.templateConfig
		if templateCfg.Name == "" {
			templateCfg.Name = common.APMPrefix
			m.supporter.log.Infof("Set setup.template.name to '%s'.", templateCfg.Name)
		}
		if templateCfg.Pattern == "" {
			templateCfg.Pattern = templateCfg.Name + "*"
			m.supporter.log.Infof("Set setup.template.pattern to '%s'.", templateCfg.Pattern)
		}
		if err := m.loadLegacyTemplate(templateCfg, m.assets.Fields(m.supporter.info.Beat)); err != nil {
			return err
		}
	}

	if !ilmFeature.load {
		return nil
	}

	var policiesLoaded []string
	var err error
	for _, ilmSupporter := range m.supporter.ilmSupporters {
		// load event type policies, respecting ILM settings
		if policiesLoaded, err = m.loadPolicy(ilmFeature, ilmSupporter, policiesLoaded); err != nil {
			return err
		}

		// (3) load event type specific template respecting index lifecycle information
		templateCfg := ilm.Template(ilmFeature.enabled, ilmFeature.overwrite,
			ilmSupporter.Alias().Name, ilmSupporter.Policy().Name)
		if err := m.loadLegacyTemplate(templateCfg, nil); err != nil {
			return err
		}

		//(4) load ilm write aliases
		//    ensure write aliases are created AFTER template creation
		if err = m.loadLegacyAlias(ilmFeature, ilmSupporter); err != nil {
			return err
		}
	}
	m.supporter.log.Info("Finished legacy index management setup.")
	return nil
}

func (m *manager) templateFeature(loadMode libidxmgmt.LoadMode) feature {
	return newFeature(m.supporter.templateConfig.Enabled, m.supporter.templateConfig.Overwrite,
		m.supporter.templateConfig.Enabled, true, loadMode)
}

func (m *manager) ilmFeature(loadMode libidxmgmt.LoadMode) feature {
	// Do not use configured `m.supporter.ilmConfig.Mode` to check if ilm is enabled.
	// The configuration might be set to `true` or `auto` but preconditions are not met,
	// e.g. ilm support by Elasticsearch
	// In these cases the supporter holds an internal state `m.supporter.st.ilmEnabled` that is set to false.
	// The originally configured value is preserved allowing to collect warnings and errors to be
	// returned to the user.

	warning := func(f feature) string {
		if !f.load {
			return msgIlmSetupDisabled
		}
		return ""
	}
	information := func(f feature) string {
		if !f.overwrite {
			return msgIlmSetupOverwriteDisabled
		}
		return ""
	}
	// m.supporter.st.ilmEnabled.Load() only returns true for cases where
	// ilm mode is configured `auto` or `true` and preconditions to enable ilm are true
	if enabled := m.supporter.st.ilmEnabled.Load(); enabled {
		f := newFeature(enabled, m.supporter.ilmConfig.Setup.Overwrite,
			m.supporter.ilmConfig.Setup.Enabled, true, loadMode)
		f.warn = warning(f)
		if m.supporter.unmanagedIdxConfig.Customized() {
			f.warn += msgIdxCfgIgnored
		}
		f.info = information(f)
		return f
	}

	var (
		err       error
		supported = true
	)
	// collect warnings when ilm is configured `auto` but it cannot be enabled
	// collect error when ilm is configured `true` but it cannot be enabled as preconditions are not met
	var warn string
	if m.supporter.ilmConfig.Mode == libilm.ModeAuto {
		if m.supporter.unmanagedIdxConfig.Customized() {
			warn = msgIlmDisabledCfg
		} else {
			warn = msgIlmDisabledES
			supported = false
		}
	} else if m.supporter.ilmConfig.Mode == libilm.ModeEnabled {
		err = errors.New(msgErrIlmDisabledES)
		supported = false
	}
	f := newFeature(false, m.supporter.ilmConfig.Setup.Overwrite, m.supporter.ilmConfig.Setup.Enabled, supported, loadMode)
	f.warn = warning(f)
	f.warn += warn
	f.info = information(f)
	f.err = err
	return f
}

func (m *manager) loadGenericComponentTemplate(templateFeature feature, name string) error {
	if !templateFeature.load {
		return nil
	}
	cfg := m.supporter.templateConfig
	cfg.Kind = template.KindComponent
	cfg.Name = name
	return m.loadTemplate(cfg, m.assets.Fields(m.supporter.info.Beat))
}

func (m *manager) loadEventComponentTemplate(ilmFeature feature, name, policyName string, withDataStream bool) error {
	if !ilmFeature.load {
		return nil
	}
	cfg := template.DefaultConfig()
	cfg.Kind = template.KindComponent
	cfg.Name = name
	cfg.Enabled = ilmFeature.load
	cfg.Overwrite = ilmFeature.overwrite
	cfg.Settings.Index = map[string]interface{}{
		"lifecycle.name": policyName,
	}
	if !withDataStream {
		cfg.Settings.Index["lifecycle.rollover_alias"] = cfg.Name
	}
	return m.loadTemplate(cfg, nil)
}

func (m *manager) loadTemplate(config template.TemplateConfig, fields []byte) error {
	if err := m.clientHandler.Load(config, m.supporter.info, fields, m.supporter.migration); err != nil {
		return errors.Wrapf(err, "error loading template %+v", config.Name)
	}
	m.supporter.log.Info("Finished template setup for %s.", config.Name)
	return nil
}

func (m *manager) loadIndexTemplate(ilmFeature feature, name, pattern string, composedOf []string, withDataStream bool) error {
	if m.supporter.ilmConfig.Setup.Kind != template.KindIndex {
		return nil
	}
	cfg := template.DefaultConfig()
	cfg.Kind = template.KindIndex
	cfg.Name = name
	cfg.Pattern = pattern
	cfg.ComposedOf = composedOf
	cfg.Enabled = ilmFeature.load
	cfg.Overwrite = ilmFeature.overwrite
	if withDataStream {
		cfg.DataStream = map[string]string{"timestamp_field": "@timestamp"}
	}
	return m.loadTemplate(cfg, nil)
}

func (m *manager) loadLegacyTemplate(cfg template.TemplateConfig, fields []byte) error {
	cfg.Kind = template.KindLegacy
	if err := m.clientHandler.Load(cfg, m.supporter.info, fields, m.supporter.migration); err != nil {
		return errors.Wrapf(err, "error loading template %+v", cfg.Name)
	}
	m.supporter.log.Infof("Finished template setup for %s.", cfg.Name)
	return nil
}

func (m *manager) loadPolicy(ilmFeature feature, ilmSupporter libilm.Supporter, policiesLoaded []string) ([]string, error) {
	policy := ilmSupporter.Policy().Name
	if !ilmFeature.enabled || utility.Contains(policy, policiesLoaded) {
		return policiesLoaded, nil
	}
	if ilmSupporter.Policy().Body == nil {
		m.supporter.log.Infof("ILM policy %s not loaded.", policy)
		return policiesLoaded, nil
	}
	_, err := ilmSupporter.Manager(m.clientHandler).EnsurePolicy(ilmFeature.overwrite)
	if err != nil {
		return policiesLoaded, err
	}
	m.supporter.log.Infof("ILM policy %s successfully loaded.", policy)
	return append(policiesLoaded, policy), nil
}
func (m *manager) loadLegacyAlias(ilmFeature feature, ilmSupporter libilm.Supporter) error {
	if !ilmFeature.enabled {
		return nil
	}
	alias := ilmSupporter.Alias().Name
	if err := ilmSupporter.Manager(m.clientHandler).EnsureAlias(); err != nil {
		if libilm.ErrReason(err) != libilm.ErrAliasAlreadyExists {
			return err
		}
		m.supporter.log.Infof("Write alias %s exists already.", alias)
		return nil
	}
	m.supporter.log.Infof("Write alias %s successfully generated.", alias)
	return nil
}

func disabledPattern() string {
	return fmt.Sprintf("apm-disabled-%v*", rand.Int63())
}
