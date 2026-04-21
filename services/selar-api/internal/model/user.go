// Package model defines the core domain types for the SELAR application.
// User represents an authenticated user with a study cohort assignment
// (control, treatment_auto, or treatment_hitl) and personalisation preferences.
package model

import "time"

// Cohort represents the study cohort assignment.
type Cohort string

const (
	CohortControl       Cohort = "control"
	CohortTreatmentAuto Cohort = "treatment_auto"
	CohortTreatmentHITL Cohort = "treatment_hitl"
)

// User represents a SELAR user account.
type User struct {
	ID             string         `json:"id"`
	Email          string         `json:"email"`
	Password       string         `json:"-"`
	Cohort         Cohort         `json:"cohort"`
	DriveConnected bool           `json:"drive_connected"`
	Preferences    map[string]any `json:"preferences"`
	CreatedAt      time.Time      `json:"created_at"`
}
