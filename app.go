package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	// "github.com/aws/aws-sdk-go/service/s3"
)

type RepoScanner struct {
	cv       CredentialValidator
	repoPath string
	report   *os.File
}

func (r *RepoScanner) ScanRepo() error {
	dirName := getRepoName(r.repoPath)
	branches, err := getAllBranches(dirName)
	if err != nil {
		fmt.Println("Error getting branches:", err)
		return err
	}

	for _, branch := range branches {
		err = r.scanBranch(branch, dirName)
		if err != nil {
			fmt.Println("Error scanning branch:", err)
		}
	}

	return nil

}

func (r *RepoScanner) scanBranch(branch, dirName string) error {
	err := switchToRef(branch, dirName)
	if err != nil {
		fmt.Println("Error switching to branch:", err)
		return err
	}

	msg := fmt.Sprintf("In branch %s\n", branch)
	fmt.Println(msg)
	r.report.Write([]byte(msg))

	commits, err := getAllCommits(dirName)
	if err != nil {
		return nil
	}
	for _, commit := range commits {
		err = r.scanCommit(commit, dirName)
		if err != nil {
			fmt.Println("Error scanning files in commit:", err)
			return err
		}
	}

	return nil
}

func (r *RepoScanner) scanCommit(commit, dirName string) error {
	err := switchToRef(commit, dirName)
	if err != nil {
		fmt.Println("Error switching to commit:", err)
		return err
	}
	msg := fmt.Sprintf("\tIn commit %s\n", commit)
	fmt.Println(msg)
	r.report.Write([]byte(msg))
	err = scanDir(dirName, r.report, r.cv)
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

func getRepoName(repoPath string) string {
	slice := strings.Split(repoPath, "/")
	folderName := slice[len(slice)-1]
	return folderName
}

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

func scanDir(dirName string, out *os.File, cv CredentialValidator) error {
	files, err := ioutil.ReadDir(dirName)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}
	for _, file := range files {
		if file.IsDir() {
			err = scanDir(dirName+"/"+file.Name(), out, cv)
			if err != nil {
				return err
			}
		} else {
			wg.Add(1)
			go func(wg *sync.WaitGroup, name string) {

				defer wg.Done()
				f, err := os.Open(dirName + "/" + name)
				if err != nil {
					log.Println(err)
					return
				}

				defer f.Close()

				if err := scanFile(cv, f, out); err != nil {
					switch err {
					case errorInvalidCreds, errorNoMatch:
						return
					default:
						log.Println(err)
						return
					}
				}

			}(&wg, file.Name())
		}

	}

	wg.Wait()
	return nil
}

func scanFile(cv CredentialValidator, in, out *os.File) error {
	fileContent, err := ioutil.ReadAll(in)
	if err != nil {
		return err
	}

	cc, err := ParseCredentials(string(fileContent), cv)
	if err != nil {
		return err
	}

	result := ""
	for _, c := range cc {
		result += fmt.Sprintf("\n\nValid IAM key found in file %s:\n\t\t\tAccess Key: %s\n\t\t\tSecret Access Key: %s\n\n", in.Name(), c.Id, c.Secret)
	}

	_, err = out.Write([]byte(result))
	if err != nil {
		return err
	}

	return nil
}

func cloneRepository(repoPath string) error {
	cmd := exec.Command("git", "clone", repoPath)
	err := cmd.Run()
	return err
}

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

	err = os.Mkdir("logs", os.ModePerm)
	if err != nil {
		// log.Fatal(err)
		fmt.Println("Error creating folder - logs:", err)
	}

	f, err := os.OpenFile("logs/"+getRepoName(repoPath)+"-result.txt", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	rs := RepoScanner{report: f, repoPath: repoPath, cv: awsValidator{}}
	// Step 2: Traverse through all files in the repository to find potential AWS IAM keys
	err = rs.ScanRepo()
	if err != nil {
		fmt.Println("Error scanning repository:", err)
	}

	err = f.Close()
	if err != nil {
		fmt.Println("Error closing file:", err)
		log.Fatal(err)
	}

	// Step 3: Clean up cloned repository
	err = os.RemoveAll(getRepoName(repoPath))
	if err != nil {
		fmt.Println("Error cleaning up cloned repository:", err)
	}
}
