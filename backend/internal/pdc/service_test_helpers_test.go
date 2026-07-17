package pdc

type testPdcEuroscope struct{}

func (testPdcEuroscope) GetMasterCallsign(int32) string                   { return "" }
func (testPdcEuroscope) GetMasterCid(int32) string                        { return "" }
func (testPdcEuroscope) SendClearedFlag(int32, string, string, bool)      {}
func (testPdcEuroscope) SendPdcStateChange(int32, string, string, string) {}
func (testPdcEuroscope) SendRoute(int32, string, string, string)          {}
func (testPdcEuroscope) SendSid(int32, string, string, string)            {}
