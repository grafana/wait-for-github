package logging

import (
	"log/slog"
)

func CheckRunAttr(value string) slog.Attr {
	return slog.String("check_run", value)
}

func StatusContextAttr(value string) slog.Attr {
	return slog.String("status_context", value)
}

func OwnerAttr(value string) slog.Attr {
	return slog.String("owner", value)
}

func RepoAttr(value string) slog.Attr {
	return slog.String("repo", value)
}

func RefAttr(value string) slog.Attr {
	return slog.String("ref", value)
}

func NameAttr(value string) slog.Attr {
	return slog.String("name", value)
}
