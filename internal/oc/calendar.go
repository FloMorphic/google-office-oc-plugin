package oc

import "encoding/json"

// Google Calendar session helpers: listing the account's calendars for the
// calendarId picker. Unlike Drive, a calendar's id IS the value the API speaks
// in (an email, "primary", or an @group.calendar.google.com id), so there is no
// name→id resolution — the picker writes the id directly and the field stays
// typable for "primary" or a pasted id.

// actionListCalendars lists the account's calendar-list entries. It backs the
// "Load calendars" picker.
const actionListCalendars = "googlecalendar.list_calendars"

// Calendar is one calendar the account can see: its id (what every event action
// wants as calendarId), its human summary, and whether it is the primary one.
type Calendar struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Primary bool   `json:"primary"`
}

// Calendars lists the account's calendars for the calendarId picker, newest
// listing order as Google returns them. Read leniently so a schema tweak
// degrades to "no calendars" rather than an error.
func (s *Session) Calendars() ([]Calendar, error) {
	raw, err := s.Do(actionListCalendars, map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []Calendar `json:"items"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out.Items, nil
}
