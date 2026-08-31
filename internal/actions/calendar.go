package actions

import (
	"strings"

	"github.com/FloMorphic/google-office-oc-plugin/internal/oc"
	"github.com/Inflowenger/go-plugin-sdk/sdkv1"
)

// Google Calendar actions. Each forwards to a googlecalendar.* OpenConnector
// action and returns the object the gateway answers with. The event-write actions
// (create/update) present a FLAT form — summary, start, end, attendees, … — and
// assemble the nested `event` object the gateway wants in the handler, so the user
// never hand-writes event JSON. calendarId is the calendar's own id (an email,
// "primary", or an @group id): the picker writes it directly, no name→id step;
// empty means the primary calendar.
//
// Every action is tagged class="calendar" so the frontend groups these ports as
// one product. Several nodes name a synthetic Method (their canvas identity) that
// differs from the OC action the handler calls (e.g. Update → patch_event).

// calendarClass stamps the shared class tag onto a Calendar action.
func calendarClass() map[string]string { return map[string]string{"class": classCalendar} }

// calendarFormByMethod lets the "Load calendars" picker rebuild the right form:
// the button posts its action's method (via Field.Picks), and the meta looks the
// form up here to turn the calendarId field into a drop-down. Keep in sync with
// the actions below and their forms in forms.go.
var calendarFormByMethod = map[string]sdkv1.FormBuilder{
	"googlecalendar.list_events":     calendarListEventsForm,
	"googlecalendar.get_event":       calendarGetEventForm,
	"googlecalendar.create_event":    calendarCreateEventForm,
	"googlecalendar.quick_add_event": calendarQuickAddForm,
	"googlecalendar.patch_event":     calendarUpdateEventForm,
	"googlecalendar.delete_event":    calendarDeleteEventForm,
	"googlecalendar.move_event":      calendarMoveEventForm,
	"googlecalendar.add_attendee":    calendarAddAttendeeForm,
	"googlecalendar.remove_attendee": calendarRemoveAttendeeForm,
}

// calendarActions is the ordered set of Calendar nodes this plugin exposes.
func (r *Registry) calendarActions() []sdkv1.Action {
	return []sdkv1.Action{
		r.calendarListCalendars(),
		r.calendarListEvents(),
		r.calendarGetEvent(),
		r.calendarCreateEvent(),
		r.calendarQuickAddEvent(),
		r.calendarUpdateEvent(),
		r.calendarDeleteEvent(),
		r.calendarMoveEvent(),
		r.calendarFindFreeSlots(),
		r.calendarAddAttendee(),
		r.calendarRemoveAttendee(),
	}
}

// calendarIDOrPrimary returns the calendar id to use, defaulting to "primary"
// when the user left the field empty — the common case, and what the API assumes.
func calendarIDOrPrimary(ref string) string {
	if ref = strings.TrimSpace(ref); ref != "" {
		return ref
	}
	return "primary"
}

// eventTime builds one end of an event's time range for the gateway's `event`
// object: an all-day event carries a {date: YYYY-MM-DD}, a timed one a
// {dateTime: RFC3339[, timeZone]}.
func eventTime(value, timeZone string, allDay bool) map[string]any {
	if allDay {
		return map[string]any{"date": value}
	}
	t := map[string]any{"dateTime": value}
	if strings.TrimSpace(timeZone) != "" {
		t["timeZone"] = timeZone
	}
	return t
}

// attendeeList turns a list of email addresses into the event object's attendees
// array ([]{email}). Blank entries are skipped; nil when there are none.
func attendeeList(emails []string) []map[string]any {
	var out []map[string]any
	for _, e := range emails {
		if e = strings.TrimSpace(e); e != "" {
			out = append(out, map[string]any{"email": e})
		}
	}
	return out
}

// ---------------------------------------------------------- list calendars --

func (r *Registry) calendarListCalendars() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.list_calendars",
		Title:       "Calendar: List calendars",
		Description: "List the calendars on the connected account (id, summary, access role) (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-multiple"},
		Tags:        calendarClass(),
		Form:        calendarListCalendarsForm,
		RequestHandler: run(r, "list calendars", func(job *sdkv1.Job, sess *oc.Session, in struct {
			ShowHidden  bool `json:"showHidden,omitempty"`
			ShowDeleted bool `json:"showDeleted,omitempty"`
		}) (map[string]any, error) {
			payload := map[string]any{}
			if in.ShowHidden {
				payload["showHidden"] = true
			}
			if in.ShowDeleted {
				payload["showDeleted"] = true
			}
			raw, err := sess.Do("googlecalendar.list_calendars", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ------------------------------------------------------------- list events --

type calendarListEventsInput struct {
	CalendarID  string `json:"calendarId,omitempty"`
	Query       string `json:"q,omitempty"`
	TimeMin     string `json:"timeMin,omitempty"`
	TimeMax     string `json:"timeMax,omitempty"`
	TimeZone    string `json:"timeZone,omitempty"`
	OrderBy     string `json:"orderBy,omitempty"`
	MaxResults  int    `json:"maxResults,omitempty"`
	ShowDeleted bool   `json:"showDeleted,omitempty"`
}

func (r *Registry) calendarListEvents() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.list_events",
		Title:       "Calendar: List events",
		Description: "List events on a calendar within a time window, optionally filtered by a search term (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-search"},
		Tags:        calendarClass(),
		Form:        calendarListEventsForm,
		RequestHandler: run(r, "list events", func(job *sdkv1.Job, sess *oc.Session, in calendarListEventsInput) (map[string]any, error) {
			payload := map[string]any{"calendarId": calendarIDOrPrimary(in.CalendarID)}
			// singleEvents expands recurring events into instances; it is also what
			// lets orderBy be "startTime", so it is always on for the common case.
			payload["singleEvents"] = true
			for k, v := range map[string]string{
				"q":        in.Query,
				"timeMin":  in.TimeMin,
				"timeMax":  in.TimeMax,
				"timeZone": in.TimeZone,
				"orderBy":  in.OrderBy,
			} {
				if strings.TrimSpace(v) != "" {
					payload[k] = v
				}
			}
			if in.MaxResults > 0 {
				payload["maxResults"] = in.MaxResults
			}
			if in.ShowDeleted {
				payload["showDeleted"] = true
			}
			raw, err := sess.Do("googlecalendar.list_events", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// --------------------------------------------------------------- get event --

type calendarGetEventInput struct {
	CalendarID string `json:"calendarId,omitempty"`
	EventID    string `json:"eventId"`
}

func (r *Registry) calendarGetEvent() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.get_event",
		Title:       "Calendar: Get event",
		Description: "Read a single event by id and return its full details (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-text"},
		Tags:        calendarClass(),
		Form:        calendarGetEventForm,
		RequestHandler: run(r, "get event", func(job *sdkv1.Job, sess *oc.Session, in calendarGetEventInput) (map[string]any, error) {
			if err := requireAll(nv("eventId", in.EventID)); err != nil {
				return nil, err
			}
			raw, err := sess.Do("googlecalendar.get_event", map[string]any{
				"calendarId": calendarIDOrPrimary(in.CalendarID),
				"eventId":    in.EventID,
			})
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ------------------------------------------------------------ create event --

type calendarCreateEventInput struct {
	CalendarID  string   `json:"calendarId,omitempty"`
	Summary     string   `json:"summary"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	AllDay      bool     `json:"allDay,omitempty"`
	TimeZone    string   `json:"timeZone,omitempty"`
	Location    string   `json:"location,omitempty"`
	Description string   `json:"description,omitempty"`
	Attendees   []string `json:"attendees,omitempty"`
	SendUpdates string   `json:"sendUpdates,omitempty"`
}

func (r *Registry) calendarCreateEvent() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.create_event",
		Title:       "Calendar: Create event",
		Description: "Create an event with a title, start/end time, optional location, description and attendees (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-plus"},
		Tags:        calendarClass(),
		Form:        calendarCreateEventForm,
		RequestHandler: run(r, "create event", func(job *sdkv1.Job, sess *oc.Session, in calendarCreateEventInput) (map[string]any, error) {
			if err := requireAll(nv("summary", in.Summary), nv("start", in.Start), nv("end", in.End)); err != nil {
				return nil, err
			}
			event := map[string]any{
				"summary": in.Summary,
				"start":   eventTime(in.Start, in.TimeZone, in.AllDay),
				"end":     eventTime(in.End, in.TimeZone, in.AllDay),
			}
			if strings.TrimSpace(in.Location) != "" {
				event["location"] = in.Location
			}
			if strings.TrimSpace(in.Description) != "" {
				event["description"] = in.Description
			}
			if att := attendeeList(in.Attendees); len(att) > 0 {
				event["attendees"] = att
			}
			payload := map[string]any{
				"calendarId": calendarIDOrPrimary(in.CalendarID),
				"event":      event,
			}
			if strings.TrimSpace(in.SendUpdates) != "" {
				payload["sendUpdates"] = in.SendUpdates
			}
			raw, err := sess.Do("googlecalendar.create_event", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// --------------------------------------------------------- quick add event --

type calendarQuickAddInput struct {
	CalendarID  string `json:"calendarId,omitempty"`
	Text        string `json:"text"`
	SendUpdates string `json:"sendUpdates,omitempty"`
}

func (r *Registry) calendarQuickAddEvent() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.quick_add_event",
		Title:       "Calendar: Quick add event",
		Description: "Create an event from a natural-language phrase, e.g. \"Lunch with Sam tomorrow 1pm\" (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-edit"},
		Tags:        calendarClass(),
		Form:        calendarQuickAddForm,
		RequestHandler: run(r, "quick add event", func(job *sdkv1.Job, sess *oc.Session, in calendarQuickAddInput) (map[string]any, error) {
			if err := requireAll(nv("text", in.Text)); err != nil {
				return nil, err
			}
			payload := map[string]any{
				"calendarId": calendarIDOrPrimary(in.CalendarID),
				"text":       in.Text,
			}
			if strings.TrimSpace(in.SendUpdates) != "" {
				payload["sendUpdates"] = in.SendUpdates
			}
			raw, err := sess.Do("googlecalendar.quick_add_event", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ------------------------------------------------------------ update event --

type calendarUpdateEventInput struct {
	CalendarID  string   `json:"calendarId,omitempty"`
	EventID     string   `json:"eventId"`
	Summary     string   `json:"summary,omitempty"`
	Start       string   `json:"start,omitempty"`
	End         string   `json:"end,omitempty"`
	AllDay      bool     `json:"allDay,omitempty"`
	TimeZone    string   `json:"timeZone,omitempty"`
	Location    string   `json:"location,omitempty"`
	Description string   `json:"description,omitempty"`
	Attendees   []string `json:"attendees,omitempty"`
	SendUpdates string   `json:"sendUpdates,omitempty"`
}

func (r *Registry) calendarUpdateEvent() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.patch_event",
		Title:       "Calendar: Update event",
		Description: "Change fields on an existing event — only the fields you set are updated (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-edit"},
		Tags:        calendarClass(),
		Form:        calendarUpdateEventForm,
		RequestHandler: run(r, "update event", func(job *sdkv1.Job, sess *oc.Session, in calendarUpdateEventInput) (map[string]any, error) {
			if err := requireAll(nv("eventId", in.EventID)); err != nil {
				return nil, err
			}
			// patch_event is a partial update: build the event object from only the
			// fields the user actually set, so untouched fields are left as they are.
			event := map[string]any{}
			if strings.TrimSpace(in.Summary) != "" {
				event["summary"] = in.Summary
			}
			if strings.TrimSpace(in.Start) != "" {
				event["start"] = eventTime(in.Start, in.TimeZone, in.AllDay)
			}
			if strings.TrimSpace(in.End) != "" {
				event["end"] = eventTime(in.End, in.TimeZone, in.AllDay)
			}
			if strings.TrimSpace(in.Location) != "" {
				event["location"] = in.Location
			}
			if strings.TrimSpace(in.Description) != "" {
				event["description"] = in.Description
			}
			if att := attendeeList(in.Attendees); len(att) > 0 {
				event["attendees"] = att
			}
			if len(event) == 0 {
				return nil, requireAll(nv("a field to update (summary, start, end, location, description, or attendees)", ""))
			}
			payload := map[string]any{
				"calendarId": calendarIDOrPrimary(in.CalendarID),
				"eventId":    in.EventID,
				"event":      event,
			}
			if strings.TrimSpace(in.SendUpdates) != "" {
				payload["sendUpdates"] = in.SendUpdates
			}
			raw, err := sess.Do("googlecalendar.patch_event", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ------------------------------------------------------------ delete event --

type calendarDeleteEventInput struct {
	CalendarID  string `json:"calendarId,omitempty"`
	EventID     string `json:"eventId"`
	SendUpdates string `json:"sendUpdates,omitempty"`
}

func (r *Registry) calendarDeleteEvent() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.delete_event",
		Title:       "Calendar: Delete event",
		Description: "Delete an event by id, optionally notifying the attendees (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-remove"},
		Tags:        calendarClass(),
		Form:        calendarDeleteEventForm,
		RequestHandler: run(r, "delete event", func(job *sdkv1.Job, sess *oc.Session, in calendarDeleteEventInput) (map[string]any, error) {
			if err := requireAll(nv("eventId", in.EventID)); err != nil {
				return nil, err
			}
			payload := map[string]any{
				"calendarId": calendarIDOrPrimary(in.CalendarID),
				"eventId":    in.EventID,
			}
			if strings.TrimSpace(in.SendUpdates) != "" {
				payload["sendUpdates"] = in.SendUpdates
			}
			raw, err := sess.Do("googlecalendar.delete_event", payload)
			if err != nil {
				return nil, err
			}
			out := object(raw)
			out["eventId"] = in.EventID
			out["deleted"] = true
			return out, nil
		}),
	}
}

// -------------------------------------------------------------- move event --

type calendarMoveEventInput struct {
	CalendarID            string `json:"calendarId,omitempty"`
	EventID               string `json:"eventId"`
	DestinationCalendarID string `json:"destinationCalendarId"`
	SendUpdates           string `json:"sendUpdates,omitempty"`
}

func (r *Registry) calendarMoveEvent() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.move_event",
		Title:       "Calendar: Move event",
		Description: "Move an event to another calendar (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-export"},
		Tags:        calendarClass(),
		Form:        calendarMoveEventForm,
		RequestHandler: run(r, "move event", func(job *sdkv1.Job, sess *oc.Session, in calendarMoveEventInput) (map[string]any, error) {
			if err := requireAll(nv("eventId", in.EventID), nv("destinationCalendarId", in.DestinationCalendarID)); err != nil {
				return nil, err
			}
			payload := map[string]any{
				"calendarId":            calendarIDOrPrimary(in.CalendarID),
				"eventId":               in.EventID,
				"destinationCalendarId": in.DestinationCalendarID,
			}
			if strings.TrimSpace(in.SendUpdates) != "" {
				payload["sendUpdates"] = in.SendUpdates
			}
			raw, err := sess.Do("googlecalendar.move_event", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ---------------------------------------------------------- find free slots --

type calendarFindFreeInput struct {
	CalendarIDs []string `json:"calendarIds,omitempty"`
	TimeMin     string   `json:"timeMin"`
	TimeMax     string   `json:"timeMax"`
	TimeZone    string   `json:"timeZone,omitempty"`
}

func (r *Registry) calendarFindFreeSlots() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.find_free_slots",
		Title:       "Calendar: Find free/busy",
		Description: "Query the free/busy ranges of one or more calendars over a time window (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-calendar-clock"},
		Tags:        calendarClass(),
		Form:        calendarFindFreeForm,
		RequestHandler: run(r, "find free/busy", func(job *sdkv1.Job, sess *oc.Session, in calendarFindFreeInput) (map[string]any, error) {
			if err := requireAll(nv("timeMin", in.TimeMin), nv("timeMax", in.TimeMax)); err != nil {
				return nil, err
			}
			// items is the list of calendars to query; default to the primary
			// calendar when the user named none. The gateway accepts bare id strings.
			items := make([]string, 0, len(in.CalendarIDs))
			for _, c := range in.CalendarIDs {
				if c = strings.TrimSpace(c); c != "" {
					items = append(items, c)
				}
			}
			if len(items) == 0 {
				items = []string{"primary"}
			}
			payload := map[string]any{
				"items":   items,
				"timeMin": in.TimeMin,
				"timeMax": in.TimeMax,
			}
			if strings.TrimSpace(in.TimeZone) != "" {
				payload["timeZone"] = in.TimeZone
			}
			raw, err := sess.Do("googlecalendar.find_free_slots", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// ------------------------------------------------------------ add attendee --

type calendarAddAttendeeInput struct {
	CalendarID    string `json:"calendarId,omitempty"`
	EventID       string `json:"eventId"`
	AttendeeEmail string `json:"attendeeEmail"`
	DisplayName   string `json:"displayName,omitempty"`
	Optional      bool   `json:"optional,omitempty"`
	SendUpdates   string `json:"sendUpdates,omitempty"`
}

func (r *Registry) calendarAddAttendee() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.add_attendee",
		Title:       "Calendar: Add attendee",
		Description: "Add an attendee to an existing event, optionally notifying them (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-account-plus"},
		Tags:        calendarClass(),
		Form:        calendarAddAttendeeForm,
		RequestHandler: run(r, "add attendee", func(job *sdkv1.Job, sess *oc.Session, in calendarAddAttendeeInput) (map[string]any, error) {
			if err := requireAll(nv("eventId", in.EventID), nv("attendeeEmail", in.AttendeeEmail)); err != nil {
				return nil, err
			}
			payload := map[string]any{
				"calendarId":    calendarIDOrPrimary(in.CalendarID),
				"eventId":       in.EventID,
				"attendeeEmail": in.AttendeeEmail,
			}
			if strings.TrimSpace(in.DisplayName) != "" {
				payload["displayName"] = in.DisplayName
			}
			if in.Optional {
				payload["optional"] = true
			}
			if strings.TrimSpace(in.SendUpdates) != "" {
				payload["sendUpdates"] = in.SendUpdates
			}
			raw, err := sess.Do("googlecalendar.add_attendee", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}

// --------------------------------------------------------- remove attendee --

type calendarRemoveAttendeeInput struct {
	CalendarID    string `json:"calendarId,omitempty"`
	EventID       string `json:"eventId"`
	AttendeeEmail string `json:"attendeeEmail"`
	SendUpdates   string `json:"sendUpdates,omitempty"`
}

func (r *Registry) calendarRemoveAttendee() sdkv1.Action {
	return sdkv1.Action{
		Method:      "googlecalendar.remove_attendee",
		Title:       "Calendar: Remove attendee",
		Description: "Remove an attendee from an existing event by email (via OpenConnector).",
		Icon:        sdkv1.Icon{Icon: "mdi-account-remove"},
		Tags:        calendarClass(),
		Form:        calendarRemoveAttendeeForm,
		RequestHandler: run(r, "remove attendee", func(job *sdkv1.Job, sess *oc.Session, in calendarRemoveAttendeeInput) (map[string]any, error) {
			if err := requireAll(nv("eventId", in.EventID), nv("attendeeEmail", in.AttendeeEmail)); err != nil {
				return nil, err
			}
			payload := map[string]any{
				"calendarId":    calendarIDOrPrimary(in.CalendarID),
				"eventId":       in.EventID,
				"attendeeEmail": in.AttendeeEmail,
			}
			if strings.TrimSpace(in.SendUpdates) != "" {
				payload["sendUpdates"] = in.SendUpdates
			}
			raw, err := sess.Do("googlecalendar.remove_attendee", payload)
			if err != nil {
				return nil, err
			}
			return object(raw), nil
		}),
	}
}
