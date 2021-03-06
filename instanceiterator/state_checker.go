// Copyright (C) 2015-Present Pivotal Software, Inc. All rights reserved.

// This program and the accompanying materials are made available under
// the terms of the under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package instanceiterator

import (
	"fmt"

	"github.com/pivotal-cf/brokerapi"
	"github.com/pivotal-cf/on-demand-service-broker/broker"
	"github.com/pivotal-cf/on-demand-service-broker/broker/services"
)

type LastOperationChecker struct {
	brokerServices BrokerServices
}

func NewStateChecker(brokerServices BrokerServices) *LastOperationChecker {
	return &LastOperationChecker{
		brokerServices: brokerServices,
	}
}

func (l *LastOperationChecker) Check(guid string, operationData broker.OperationData) (services.BOSHOperation, error) {
	lastOperation, err := l.brokerServices.LastOperation(guid, operationData)
	if err != nil {
		return services.BOSHOperation{}, fmt.Errorf("error getting last operation: %s", err)
	}

	boshOperation := services.BOSHOperation{Data: operationData, Description: lastOperation.Description}

	switch lastOperation.State {
	case brokerapi.Failed:
		boshOperation.Type = services.OperationFailed
	case brokerapi.Succeeded:
		boshOperation.Type = services.OperationSucceeded
	case brokerapi.InProgress:
		boshOperation.Type = services.OperationAccepted
	default:
		return services.BOSHOperation{}, fmt.Errorf("unknown state from last operation: %s", lastOperation.State)
	}

	return boshOperation, nil
}
