/*
Copyright 2026.

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

package controller

import (
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ErrorClass controls whether controller-runtime may retry an operation.
type ErrorClass string

const (
	// ErrorClassTransient identifies failures that may succeed when retried.
	ErrorClassTransient ErrorClass = "transient"
	// ErrorClassPermanent identifies invalid or forbidden operations that need a new event.
	ErrorClassPermanent ErrorClass = "permanent"
)

// ReconcileError carries a bounded reason and retry classification.
type ReconcileError struct {
	Class  ErrorClass
	Reason string
	Err    error
}

func (e *ReconcileError) Error() string {
	return fmt.Sprintf("%s reconciliation failure (%s): %v", e.Class, e.Reason, e.Err)
}

// Unwrap preserves Kubernetes API error inspection without copying it to status.
func (e *ReconcileError) Unwrap() error {
	return e.Err
}

func newReconcileError(class ErrorClass, reason string, err error) *ReconcileError {
	return &ReconcileError{Class: class, Reason: reason, Err: err}
}

func classifyAPIError(reason string, err error) *ReconcileError {
	if err == nil {
		return nil
	}
	if apierrors.IsConflict(err) || apierrors.IsServerTimeout(err) || apierrors.IsTimeout(err) ||
		apierrors.IsTooManyRequests(err) || apierrors.IsServiceUnavailable(err) {
		return newReconcileError(ErrorClassTransient, reason, err)
	}
	if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) || apierrors.IsForbidden(err) ||
		apierrors.IsUnauthorized(err) || apierrors.IsMethodNotSupported(err) {
		return newReconcileError(ErrorClassPermanent, reason, err)
	}
	// Unknown transport and API failures are retried. It is safer to bound a
	// retry than to permanently drop work on an unrecognized transient outage.
	return newReconcileError(ErrorClassTransient, reason, err)
}

func errorClass(err error) ErrorClass {
	var reconcileErr *ReconcileError
	if errors.As(err, &reconcileErr) {
		return reconcileErr.Class
	}
	return ErrorClassTransient
}
