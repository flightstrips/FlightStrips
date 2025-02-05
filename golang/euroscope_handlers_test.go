package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_euroscopehandlerAuthentication(t *testing.T) {

	server := Server{
		nil,
		nil,
	}

	/*	emptyTokenEvent := EuroscopeAuthenticationEvent{
			Type:  EuroscopeAuthentication,
			Token: "",
		}

		_, err := server.euroscopehandlerAuthentication(emptyTokenEvent)
		assert.Error(t, err)*/

	correctToken := "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IkV3NlhiWEoxTHN6UWtwY2FxeE1OdiJ9.eyJpc3MiOiJodHRwczovL2Rldi14ZDB1ZjRzZDF2MjdyOHRnLmV1LmF1dGgwLmNvbS8iLCJzdWIiOiJvYXV0aDJ8dmF0c2ltLWRldnwxMDAwMDAwNSIsImF1ZCI6WyJiYWNrZW5kIiwiaHR0cHM6Ly9kZXYteGQwdWY0c2QxdjI3cjh0Zy5ldS5hdXRoMC5jb20vdXNlcmluZm8iXSwiaWF0IjoxNzM4NjE1NzA4LCJleHAiOjE3Mzg3MDIxMDgsInNjb3BlIjoib3BlbmlkIHByb2ZpbGUgb2ZmbGluZV9hY2Nlc3MiLCJhenAiOiJsTWZxQkRraURrUG5jZ3FCOWxMWFNqOTB3cjUxejNDaSJ9.SOQVdyVG0Ok2ytPvbHFu0uWlG8d75BxtKA82iek9mq0H0yFgK2T-JXZINdSissGSjAlFAejuG3IVhkRFIiOSzaval6ajXO4750nhmqurZrCccW1k8-lUiknNcPcOsLwvg83XnSYJgLAGQxqVNPsfP9Xf76GdN3fxQ-zPiErOy0Y-lKYrzaMoRWRYp_CiMEvAAIn--sFruvme0yuZfv4XDeH9sMtKTJ-iQ70lM0U6oPcxUEr444BIBUEriqwGwdUhZbnno01MpVwAabMP4A-4pXFRxUvy9CkkVdjl1xxDRyjBD22v2SizPWMuB7dsBvwgDD9I7kHB6MUMb6ysVimDsA"

	correctTokenEvent := EuroscopeAuthenticationEvent{
		Type:  EuroscopeAuthentication,
		Token: correctToken,
	}

	token, err := server.euroscopehandlerAuthentication(correctTokenEvent)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

}
