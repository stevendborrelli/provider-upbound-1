/*
Copyright 2023 Upbound Inc.

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

package teams

import "github.com/upbound/up-sdk-go/service/common"

type GetResponse struct {
	common.DataSet `json:"data"`
}

type CreateParameters struct {
	Name           string `json:"name"`
	OrganizationID uint   `json:"organizationId"`
}

type CreateResponse struct {
	ID string `json:"id"`
}
