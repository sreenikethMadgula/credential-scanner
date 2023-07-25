package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
)

const awsKeyPattern = `(?m)(?i)AKIA[0-9A-Z]{16}\s+\S{40}|AWS[0-9A-Z]{38}\s+?\S{40}`

type awsValidator struct{}

func (a awsValidator) Match(content string) ([]CloudCredentials, error) {
	res := []CloudCredentials{}
	regex := regexp.MustCompile(awsKeyPattern)

	matches := regex.FindAllString(string(content), -1)
	for _, match := range matches {
		matchArr := regexp.MustCompile(`[^\S]+`).Split(match, 2)
		res = append(res, CloudCredentials{
			Id:     matchArr[0],
			Secret: matchArr[1],
		})
	}

	return res, nil

}

func (a awsValidator) Validate(c CloudCredentials) bool {
	return validateIAMKeys(c.Id, c.Secret)
}

func isValidIAMKey(accessKeyID string, secretAccessKey string) bool {
	// return strings.HasPrefix(iamKey, "AKIA")
	return validateIAMKeys(accessKeyID, secretAccessKey)
}

// Clone the repository locally; return any cloning errors

func validateIAMKeys(accessKeyID, secretAccessKey string) bool {
	// Create a new AWS session with the IAM keys
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("ap-south-1"),
		Credentials: credentials.NewStaticCredentials(accessKeyID, secretAccessKey, ""),
	})
	if err != nil {
		fmt.Println("Error creating AWS session:", err)
		return false
	}

	// Create a new iam service client using the session
	svc := iam.New(sess)

	// Perform a basic API call to check the IAM keys' validity
	d, err := svc.ListGroups(&iam.ListGroupsInput{})
	if err != nil {
		// fmt.Println("Invalid IAM keys:", err)

		// InvalidClientTokenId error occurs for invalid keys.
		// If keys are valid, if the role doesn't have permission
		// to list groups, it returns an AccessDenied error
		if strings.Contains(err.Error(), "InvalidClientTokenId") {
			return false
		}
		return true
	}

	fmt.Print(d)

	// IAM keys are valid
	return true
}
