// Package mocks contains generated mocks for the polling ports.
package mocks

//go:generate go run go.uber.org/mock/mockgen@v0.4.0 -source=../internal/domain/ports/submission.go -destination=submission_mock.go -package=mocks
//go:generate go run go.uber.org/mock/mockgen@v0.4.0 -source=../internal/domain/ports/polling.go -destination=polling_mock.go -package=mocks
//go:generate go run go.uber.org/mock/mockgen@v0.4.0 -source=../internal/domain/ports/classroom_course_repository.go -destination=classroom_course_mock.go -package=mocks
//go:generate go run go.uber.org/mock/mockgen@v0.4.0 -source=../internal/domain/ports/task_repository.go -destination=task_mock.go -package=mocks
//go:generate go run go.uber.org/mock/mockgen@v0.4.0 -source=../internal/domain/ports/participant.go -destination=participant_mock.go -package=mocks
//go:generate go run go.uber.org/mock/mockgen@v0.4.0 -source=../internal/domain/ports/document_parser.go -destination=document_parser_mock.go -package=mocks
