package ppdd

import "os"

func readFixture(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}
