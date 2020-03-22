package xray

import "os"

const (
	lambdaInitializedDir = "/tmp/.aws-xray"
	lambdaInitializedFile = "initialized"
)

func init() {
	if os.Getenv("LAMBDA_TASK_ROOT") == "" {
		return
	}
	err := os.MkdirAll(lambdaInitializedDir, 0755)
	if err != nil {
		log.Printf("failed to create %s: %v", lambdaInitializedDir, err)
		return
	}
	name := filepath.Join(lambdaInitializedDir, lambdaInitializedFile)
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("failed to create %s: %v", name, err)
		return
	}
	f.Close()

	now := time.Now()
	if err := os.Chtimes(name, now, now); err != nil {
		log.Printf("failed to change times of %s: %v", name, err)
		return
	}
}
