package main

// The browser sends only counts from the part before @. The address remains in
// the browser and the server stores only the classification for this policy.
type merchantEmailShape struct {
	Length      int `json:"length"`
	Letters     int `json:"letters"`
	Digits      int `json:"digits"`
	Transitions int `json:"transitions"`
}

func (s merchantEmailShape) valid() bool {
	return s.Length >= 1 && s.Length <= 64 &&
		s.Letters >= 0 && s.Letters <= s.Length && s.Digits >= 0 && s.Digits <= s.Length-s.Letters &&
		s.Transitions >= 0 && s.Transitions <= max(0, min(s.Letters+s.Digits-1, 2*min(s.Letters, s.Digits)))
}

// This conservative check identifies a single ASCII alphanumeric local part
// with repeatedly interleaved letters and digits, such as a generated handle.
func (s *merchantEmailShape) randomLocalPart() *bool {
	if s == nil || !s.valid() {
		return nil
	}
	random := s.Length >= 12 && s.Letters >= 4 && s.Digits >= 4 &&
		s.Letters+s.Digits == s.Length && s.Transitions >= 4
	return &random
}

func disposableEmailEvidence(domain string) *bool {
	if domain == "" {
		return nil
	}
	domains := disposableEmailDomains()
	if domains == nil {
		return nil
	}
	matches, _ := domains.Lookup(domain)
	disposable := matches != ""
	return &disposable
}
