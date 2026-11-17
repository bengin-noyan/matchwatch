package domain

// ValidTaxID, s'nin kontrol hanesi doğru, 10 haneli bir VKN ya da 11 haneli bir TCKN olup olmadığını söyler (VKN için
// GİB'in algoritması, TCKN için 10. ve 11. hane kuralı).
func ValidTaxID(s string) bool {
	d := make([]int, len(s))
	for i, c := range s {
		if c < '0' || c > '9' {
			return false
		}
		d[i] = int(c - '0')
	}
	switch len(d) {
	case 10:
		sum := 0
		for i := 0; i < 9; i++ {
			if t := (d[i] + 9 - i) % 10; t != 0 {
				p := (t << (9 - i)) % 9
				if p == 0 {
					p = 9
				}
				sum += p
			}
		}
		return (10-sum%10)%10 == d[9]
	case 11:
		odd, even, all := d[0]+d[2]+d[4]+d[6]+d[8], d[1]+d[3]+d[5]+d[7], 0
		for _, x := range d[:10] {
			all += x
		}
		return d[0] != 0 && ((odd*7-even)%10+10)%10 == d[9] && all%10 == d[10]
	}
	return false
}
