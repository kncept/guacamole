package encryption

func XorWithKey(data, key string) string {
	var result []byte
	for i := 0; i < len(data); i++ {
		result = append(result, data[i]^key[i%len(key)])
	}
	return string(result)
}
