package navigator

import "testing"

func TestSameRootDomain(t *testing.T) {
	data := []struct {
		domainA string
		domainB string
		same    bool
	}{
		{"hwllo.test.nitrous.io", "test.nitrous.io", true},
		{"test.nitrous.io", "test.nitrous.io", true},
		{"test.nitrous.io", "test.NiTrous.IO", true},
		{"test.nitrouspro.io", "test.nitrous.io", false},
		{"test.nitrouspro.io", "test.nitrouspro.com", false},
	}

	for _, d := range data {
		if SameRootDomain(d.domainA, d.domainB) != d.same {
			t.Fatal("failed to assert domains %v", d)
		}
	}
}
