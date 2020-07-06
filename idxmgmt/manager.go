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
	"strings"

	"github.com/pkg/errors"

	libidxmgmt "github.com/elastic/beats/v7/libbeat/idxmgmt"
	libilm "github.com/elastic/beats/v7/libbeat/idxmgmt/ilm"
	libtemplate "github.com/elastic/beats/v7/libbeat/template"

	"github.com/elastic/apm-server/idxmgmt/common"
	"github.com/elastic/apm-server/utility"
)

const (
	msgErrIlmDisabledES          = "automatically disabled ILM as not supported by configured Elasticsearch"
	msgIlmDisabledES             = "Automatically disabled ILM as configured Elasticsearch not eligible for auto enabling."
	msgIlmDisabledCfg            = "Automatically disabled ILM as custom index settings configured."
	msgIdxCfgIgnored             = "Custom index configuration ignored when ILM is enabled."
	msgIlmSetupDisabled          = "Manage ILM setup is disabled. "
	msgIlmSetupOverwriteDisabled = "Overwrite ILM setup is disabled. "
	msgTemplateSetupDisabled     = "Template loading is disabled. "

	patternSuffix         = "*"
	disabledSuffix        = "-disabled"
	disabledPatternSuffix = disabledSuffix + patternSuffix
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
	// prepare template and ILM feature
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

	// setup index management
	if ilmFeature.enabled {
		return m.setupManaged(templateFeature, ilmFeature)
	}
	return m.setupUnmanaged(templateFeature, ilmFeature)
}

func (m *manager) setupManaged(templateFeature, ilmFeature feature) error {
	if !templateFeature.load && !ilmFeature.load {
		m.supporter.log.Infof("ILM is enabled but setup is disabled. For full setup " +
			"ensure `apm-server.ilm.setup.enabled` and `setup.template.enabled` are set to `true`. ")
		return nil
	}

	// TODO(simitt): add tests for template name and pattern
	// disable generic apm template matching against all `apm*` indices,
	// by setting the index pattern to `apm-disabled*`
	name := m.supporter.templateConfig.Name
	pattern := strings.TrimRight(m.supporter.templateConfig.Pattern, patternSuffix)
	if name == "" {
		name = common.APMPrefix
	}
	if pattern == "" {
		pattern = name
	}
	pattern += disabledPatternSuffix
	if err := m.loadGenericTemplate(templateFeature, ilmFeature, name, pattern); err != nil {
		return err
	}

	//TODO(simitt): how to handle setup.template.enabled vs. apm-server.ilm.setup.enabled?
	// ensure sourcemap and onboarding templates are loaded
	for _, kind := range []string{"onboarding", "sourcemap"} {
		name := fmt.Sprintf("%s-%s", common.APMPrefix, kind)
		m.loadGenericTemplate(templateFeature, ilmFeature, name, name+patternSuffix)
	}

	// ensure event templates, policies and rollover_aliases are loaded
	var policiesLoaded []string
	for _, ilmSupporter := range m.supporter.ilmSupporters {
		// load event specific templates
		if err := m.loadEventTemplate(templateFeature, ilmFeature, ilmSupporter); err != nil {
			return err
		}

		if ilmFeature.load {
			// load policy per event type
			var err error
			if policiesLoaded, err = m.loadPolicy(ilmFeature, ilmSupporter, policiesLoaded); err != nil {
				return err
			}

			// load write alias per event type AFTER the template has been created,
			// as this step also automatically creates an index, it is important the matching templates are already there
			if err = m.loadAlias(ilmFeature, ilmSupporter); err != nil {
				return err
			}
		}
	}

	m.supporter.log.Info("Finished managed index setup.")
	return nil
}

//TODO(simitt): how to handle `setup.template.overwrite` vs. `apm-server.ilm.setup.overwrite`
func (m *manager) setupUnmanaged(templateFeature, ilmFeature feature) error {
	// TODO(simitt): add tests for template name and pattern
	// load generic apm template matching against all `apm*` indices (default)
	// if not customized, set the APM template name and pattern to the default
	name := m.supporter.templateConfig.Name
	if name == "" {
		name = common.APMPrefix
		m.supporter.log.Infof("Set setup.template.name to '%s'.", m.supporter.templateConfig.Name)
	}
	pattern := m.supporter.templateConfig.Pattern
	if pattern == "" {
		pattern = name + patternSuffix
		m.supporter.log.Infof("Set setup.template.pattern to '%s'.", m.supporter.templateConfig.Pattern)
	}
	if err := m.loadGenericTemplate(templateFeature, ilmFeature, name, pattern); err != nil {
		return err
	}

	// ensure sourcemap and onboarding templates are loaded with a non-matching index pattern
	for _, kind := range []string{"onboarding", "sourcemap"} {
		name := fmt.Sprintf("%s-%s", common.APMPrefix, kind)
		m.loadGenericTemplate(templateFeature, ilmFeature, name, name+disabledPatternSuffix)
	}

	// ensure event templates are loaded with non-matching index patterns
	for _, ilmSupporter := range m.supporter.ilmSupporters {
		// load event specific templates
		if err := m.loadEventTemplate(templateFeature, ilmFeature, ilmSupporter); err != nil {
			return err
		}
	}

	m.supporter.log.Info("Finished unmanaged index setup.")
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

func (m *manager) loadGenericTemplate(templateFeature, ilmFeature feature, name, pattern string) error {
	if !templateFeature.load && !ilmFeature.load {
		return nil
	}
	templateConfig := m.supporter.templateConfig
	templateConfig.Name = name
	templateConfig.Pattern = pattern
	if ilmFeature.overwrite {
		templateConfig.Overwrite = ilmFeature.overwrite
	}
	return m.loadTemplate(templateConfig, m.assets.Fields(m.supporter.info.Beat))
}

func (m *manager) loadEventTemplate(templateFeature, ilmFeature feature, ilmSupporter libilm.Supporter) error {
	// if neither setup.template.enabled nor apm-server.ilm.setup.enabled are `true` do not load event specific templates
	if !templateFeature.load && !ilmFeature.load {
		return nil
	}

	var templateConfig libtemplate.TemplateConfig
	var fields []byte
	name := ilmSupporter.Alias().Name
	if templateFeature.load {
		templateConfig = m.supporter.templateConfig
		//TODO(simitt): use event type rather than name for logging info
		m.supporter.log.Infof("Add mappings to template %v.", name)
		fields = m.assets.Fields(m.supporter.info.Beat)
	} else {
		templateConfig = libtemplate.DefaultConfig()
	}

	// the index pattern is only set to a matching pattern when ILM is enabled
	templateConfig.Name = name
	pattern := ilmSupporter.Alias().Name
	if !ilmFeature.enabled {
		pattern += disabledSuffix
	}
	templateConfig.Pattern = pattern + patternSuffix
	if ilmFeature.load {
		templateConfig.Enabled = true
	}
	if ilmFeature.overwrite {
		templateConfig.Overwrite = ilmFeature.overwrite
	}

	templateConfig.Settings.Index = map[string]interface{}{
		"lifecycle.name":           ilmSupporter.Policy().Name,
		"lifecycle.rollover_alias": name,
	}
	return m.loadTemplate(templateConfig, fields)
}
func (m *manager) loadTemplate(config libtemplate.TemplateConfig, fields []byte) error {
	if err := m.clientHandler.Load(config, m.supporter.info, fields, m.supporter.migration); err != nil {
		return errors.Wrapf(err, "error loading template %+v", config.Name)
	}
	m.supporter.log.Info("Finished template setup for %s.", config.Name)
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

func (m *manager) loadAlias(ilmFeature feature, ilmSupporter libilm.Supporter) error {
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
