package observability

import "github.com/getsentry/sentry-go"

// RedactSentryEvent is the centralized before-send policy for error telemetry.
// Request bodies, queries, cookies, and direct personal fields are omitted;
// messages and structured diagnostic maps pass through the shared redactor.
func RedactSentryEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	event.Message = RedactText(event.Message)
	event.Transaction = RedactText(event.Transaction)
	for i := range event.Exception {
		event.Exception[i].Value = RedactText(event.Exception[i].Value)
		if event.Exception[i].Mechanism != nil {
			event.Exception[i].Mechanism.Data = redactedMap(event.Exception[i].Mechanism.Data)
		}
	}
	for _, breadcrumb := range event.Breadcrumbs {
		breadcrumb.Message = RedactText(breadcrumb.Message)
		breadcrumb.Data = redactedMap(breadcrumb.Data)
	}
	event.Extra = redactedMap(event.Extra)
	for key, context := range event.Contexts {
		event.Contexts[key] = redactedMap(context)
	}
	for key, value := range event.Tags {
		event.Tags[key] = RedactText(value)
	}
	if event.Request != nil {
		event.Request.URL = RedactText(event.Request.URL)
		event.Request.QueryString = ""
		if event.Request.Data != "" {
			event.Request.Data = RedactedValue
		}
		if event.Request.Cookies != "" {
			event.Request.Cookies = RedactedValue
		}
		for key, value := range event.Request.Headers {
			if IsSensitiveKey(key) {
				event.Request.Headers[key] = RedactedValue
			} else {
				event.Request.Headers[key] = RedactText(value)
			}
		}
		event.Request.Env = redactedStringMap(event.Request.Env)
	}
	event.User.Email = ""
	event.User.Username = ""
	event.User.Name = ""
	event.User.IPAddress = ""
	event.User.Data = redactedStringMap(event.User.Data)
	return event
}

func redactedMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	if output, ok := RedactValue(input).(map[string]interface{}); ok {
		return output
	}
	return map[string]interface{}{"redaction_status": RedactedValue}
}

func redactedStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		if IsSensitiveKey(key) {
			output[key] = RedactedValue
		} else {
			output[key] = RedactText(value)
		}
	}
	return output
}
