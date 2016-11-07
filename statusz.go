package main

import (
	"gopkg.in/macaron.v1"
	"net/http"
	"os"
	"regexp"
)

const service = "SpongeDownloads"

func readStatus() interface{} {
	buildName := os.Getenv("OPENSHIFT_BUILD_NAME")
	if buildName == "" {
		return nil
	}

	buildNameRe := regexp.MustCompile(`^(.+)-([0-9]+)$`)
	buildNamePieces := buildNameRe.FindStringSubmatch(buildName)
	jobName := buildNamePieces[1]
	buildNum := buildNamePieces[2]
	buildTag := os.Getenv("OPENSHIFT_BUILD_NAMESPACE") + "/" + os.Getenv("OPENSHIFT_BUILD_NAME")

	return map[string]string{
		"BUILD_NUMBER": buildNum,
		"GIT_BRANCH":   os.Getenv("OPENSHIFT_BUILD_REFERENCE"),
		"GIT_COMMIT":   os.Getenv("OPENSHIFT_BUILD_COMMIT"),
		"JOB_NAME":     jobName,
		"BUILD_TAG":    buildTag,
		"SPONGE_ENV":   os.Getenv("SPONGE_ENV"),
		"SERVICE":      service,
	}
}

func statuszHandler() macaron.Handler {
	status := readStatus()
	if status == nil {
		return nil
	}

	return func(ctx macaron.Render) {
		ctx.JSON(http.StatusOK, status)
	}
}
