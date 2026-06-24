package service

import (
	"errors"
)

var (
	ErrNotFound             = errors.New("resource not found")
	ErrAlreadyExists        = errors.New("resource already exists")
	ErrForbidden            = errors.New("action forbidden")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrInvalidInput         = errors.New("invalid input")
	ErrInternal             = errors.New("internal error")
	ErrMeetupFinished       = errors.New("meetup already finished")
	ErrOrganizerCannotLeave = errors.New("organizer cannot leave own meetup")
	ErrChatReadOnly         = errors.New("chat is read-only")
)
