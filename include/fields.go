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

// Code generated by beats/dev-tools/cmd/asset/asset.go - DO NOT EDIT.

package include

import (
	"github.com/elastic/beats/libbeat/asset"
)

func init() {
	if err := asset.SetFields("apm-server", "fields.yml", Asset); err != nil {
		panic(err)
	}
}

// Asset returns asset data
func Asset() string {
	return "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"
}
