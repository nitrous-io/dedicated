package dotest

type counter int

func (c *counter) next() int {
	i := int(*c)
	(*c)++
	return i
}
