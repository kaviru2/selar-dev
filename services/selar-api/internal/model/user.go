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
	DisplayName    string         `json:"display_name"`
	Password       string         `json:"-"`
	Cohort         Cohort         `json:"cohort"`
	DriveConnected bool           `json:"drive_connected"`
	Preferences    map[string]any `json:"preferences"`
	CreatedAt      time.Time      `json:"created_at"`
	// Role is "user" or "admin"; admin routes re-read it from the database.
	Role string `json:"role"`
	// GroupLabel is a neutral admin-only label (e.g. a study condition). It is
	// never returned to the user themselves so participants cannot see it.
	GroupLabel string `json:"-"`
	// Optional in-app research-consent state for usage analytics.
	ConsentedAt      *time.Time `json:"consented_at"`
	ConsentVersion   *string    `json:"consent_version"`
	ConsentDecidedAt *time.Time `json:"consent_decided_at"`
}
