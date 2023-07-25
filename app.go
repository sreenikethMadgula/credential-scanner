package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	// "github.com/aws/aws-sdk-go/service/s3"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: go run main.go <path_to_git_repository>")
		return
	}

	repoPath := os.Args[1]

	// Step 1: Clone the Git repository locally
	err := cloneRepository(repoPath)
	if err != nil {
		fmt.Println("Error cloning the repository:", err)
		return
	}

	// Get name of local folder
	dirName := getRepoName(repoPath)

	err = os.Mkdir("logs", os.ModePerm)
	if err != nil {
		// log.Fatal(err)
		fmt.Println("Error creating folder - logs:",err)
	}

	f, err := os.OpenFile("logs/"+dirName+"-result.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	// Step 2: Traverse through all files in the repository to find potential AWS IAM keys
	err = scanRepository(dirName, f)
	if err != nil {
		fmt.Println("Error scanning repository:", err)
	}

	err = f.Close()
	if err != nil {
		fmt.Println("Error closing file:", err)
		log.Fatal(err)
	}

	// Step 3: Clean up cloned repository
	err = os.RemoveAll(dirName)
	if err != nil {
		fmt.Println("Error cleaning up cloned repository:", err)
	}
}

func scanRepository(dirName string, f *os.File) error {
	branches, err := getAllBranches(dirName)
	if err != nil {
		fmt.Println("Error getting branches:", err)
		return err
	}

	for _, branch := range branches {

		err = scanBranch(branch, dirName, f)
		if err != nil {
			fmt.Println("Error scanning branch:", err)
			return err
		}
	}
	return nil
}

func scanBranch(branch string, dirName string, f *os.File) error {
	err := switchToRef(branch, dirName)
	if err != nil {
		fmt.Println("Error switching to branch:", err)
		return err
	}

	msg := fmt.Sprintf("In branch %s\n", branch)
	fmt.Println(msg)
	f.Write([]byte(msg))

	commits, err := getAllCommits(dirName)
	if err != nil {
		return nil
	}
	for _, commit := range commits {
		err = scanCommit(commit, dirName, f)
		if err != nil {
			fmt.Println("Error scanning files in commit:", err)
			return err
		}
	}
	return nil
}

// func scanRepository(dirName string, f *os.File) error {
// 	files, err := ioutil.ReadDir(dirName)
// 	if err != nil {
// 		return err
// 	}

// 	for _, file := range files {
// 		if file.IsDir() {
// 			err = scanRepository(dirName+"/"+file.Name(), f)
// 			if err != nil {
// 				return err
// 			}
// 		} else {
// 			// Check for potential IAM keys in the file content
// 			err = checkIAMKeys(dirName+"/"+file.Name(), f)
// 			if err != nil {
// 				return err
// 			}
// 		}
// 	}

// 	return nil
// }

func scanCommit(commit string, dirName string, f *os.File) error {
	err := switchToRef(commit, dirName)
	if err != nil {
		fmt.Println("Error switching to commit:", err)
		return err
	}
	msg := fmt.Sprintf("\tIn commit %s\n", commit)
	fmt.Println(msg)
	f.Write([]byte(msg))

	// files, err := ioutil.ReadDir(dirName)
	// if err != nil {
	// 	return err
	// }

	// for _, file := range files {
	// 	if file.IsDir() {
	// 		err = scanCommit(commit, dirName+"/"+file.Name(), f)
	// 		if err != nil {
	// 			return err
	// 		}
	// 	} else {
	// 		// Check for potential IAM keys in the file content
	// 		err = checkIAMKeys(dirName+"/"+file.Name(), f)
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	// }
	err = scanDir(dirName, f)
	if err != nil {
		fmt.Println("Error scanning directory:", err)
	}

	// switch back to HEAD
	cmd := exec.Command("git", "-C", dirName, "switch", "-")
	err = cmd.Run()
	if err != nil {
		fmt.Println("Error switching back to HEAD:", err)
		return err
	}

	return nil
}

func scanDir(dirName string, f *os.File) error {
	files, err := ioutil.ReadDir(dirName)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			err = scanDir(dirName+"/"+file.Name(), f)
			if err != nil {
				return err
			}
		} else {
			// Check for potential IAM keys in the file content
			err = checkIAMKeys(dirName+"/"+file.Name(), f)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func checkIAMKeys(filePath string, f *os.File) error {
	fileContent, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Regular expression to find potential IAM keys in the file content
	// AWS IAM keys have the format: "AKIA" followed by 20 alphanumeric characters
	// and "AWS" followed by 40 alphanumeric characters.
	// regex := regexp.MustCompile(`(?m)(?i)AKIA[0-9A-Z]{16}|AWS[0-9A-Z]{38}`)
	regex := regexp.MustCompile(`(?m)(?i)AKIA[0-9A-Z]{16}\s+\S{40}|AWS[0-9A-Z]{38}\s+?\S{40}`)

	matches := regex.FindAllString(string(fileContent), -1)
	for _, match := range matches {
		matchArr := regexp.MustCompile(`[^\S]+`).Split(match, 2)
		accessKeyID, secretAccessKey := matchArr[0], matchArr[1]
		// Step 4: Validate each IAM key by invoking a basic AWS API
		result := ""
		if isValidIAMKey(accessKeyID,secretAccessKey) {
			result = fmt.Sprintf("\t\tValid IAM key found in file %s:\n\t\t\tAccess Key: %s\n\t\t\tSecret Access Key: %s\n\n", filePath, accessKeyID, secretAccessKey)
			fmt.Printf(result)
			_, err := f.Write([]byte(result))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func isValidIAMKey(accessKeyID string, secretAccessKey string) bool {
	// return strings.HasPrefix(iamKey, "AKIA")
	return validateIAMKeys(accessKeyID, secretAccessKey)
}

// Clone the repository locally; return any cloning errors
func cloneRepository(repoPath string) error {
	cmd := exec.Command("git", "clone", repoPath)
	err := cmd.Run()
	return err
}

func getRepoName(repoPath string) string {
	slice := strings.Split(repoPath, "/")
	folderName := slice[len(slice)-1]
	return folderName
}

// func getFormattedResultString()

// func writeToFile(result string, repoName string) {
// 	data := []byte(result)
// 	os.WriteFile("logs/"+repoName+"-result.txt", data, 0644)
// }

func getAllBranches(dirName string) ([]string, error) {
	cmd := exec.Command("git", "-C", "Devops-Node", "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	branches := strings.Split(strings.TrimSpace(string(output)), "\n")
	return branches, nil
}

func switchToRef(ref string, dirName string) error {
	cmd := exec.Command("git", "-C", dirName, "checkout", ref)
	err := cmd.Run()
	if err != nil {
		fmt.Println("Error switching to ref:", ref, err)
		return err
	}
	return nil
}

func getAllCommits(dirName string) ([]string, error) {
	cmd := exec.Command("git", "-C", dirName, "rev-list", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	commits := strings.Split(strings.TrimSpace(string(output)), "\n")
	return commits, nil
}

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
