package must

import "io/ioutil"

func ReadFile(filename string) []byte {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	return b
}
