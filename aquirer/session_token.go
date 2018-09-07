package aquirer

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/kreuzwerker/awsu/config"
	"github.com/kreuzwerker/awsu/generator"
	"github.com/kreuzwerker/awsu/log"
)

const (
	errSessionTokenWithoutMFA           = "failed to get session token on unsuitable profiles: at least one MFA must be configured"
	errSessionTokenOnUnsuitableProfiles = "failed to get session token on unsuitable profiles: at least one long-term keypair must be configured"
)

type SessionToken struct {
	Duration      time.Duration
	Generator     generator.Name
	Grace         time.Duration
	MFASerial     string // override or set explicitly
	Profiles      []*config.Profile
	_serialNumber string
}

func (s *SessionToken) serialNumber() string {

	if s._serialNumber != "" {
		return s._serialNumber
	}

	if s._serialNumber = s.MFASerial; s._serialNumber != "" {
		log.Log("using explicitly supplied MFA serial")
		return s._serialNumber
	}

	// find the MFA
	for _, profile := range s.Profiles {

		if profile != nil && profile.MFASerial != "" {
			s._serialNumber = profile.MFASerial
			log.Log("using %q profile for MFA serial", profile.Name)
			return s._serialNumber
		}

	}

	// TODO: try autodetection as a last resort OR just don't get a session token?

	return s._serialNumber

}

func (s *SessionToken) Credentials(sess *session.Session) (*Credentials, error) {

	var (
		client       = sts.New(sess)
		lt           = s.Profile()
		serialNumber = s.serialNumber()
	)

	log.Log("getting session token for profile %q and serial %q", lt.Name, serialNumber)

	generator, ok := generator.Generators[s.Generator]

	if !ok {
		return nil, fmt.Errorf("unknown generator %q", s.Generator)
	}

	token, err := generator(serialNumber)

	if err != nil {
		return nil, err
	}

	res, err := client.GetSessionToken(&sts.GetSessionTokenInput{
		DurationSeconds: aws.Int64(int64(s.Duration.Seconds())),
		SerialNumber:    &serialNumber,
		TokenCode:       &token,
	})

	if err != nil {
		return nil, err
	}

	creds := newShortTermCredentials(
		lt.Name,
		*res.Credentials.AccessKeyId,
		*res.Credentials.SecretAccessKey,
		*res.Credentials.SessionToken,
		time.Now().Add(s.Duration).Add(s.Grace*-1),
	)

	return creds, nil

}

func (s *SessionToken) IsCacheable() bool {
	return true
}

func (s *SessionToken) Name() string {
	return "session_token"
}

func (s *SessionToken) Profile() *config.Profile {

	for _, profile := range s.Profiles {

		if profile != nil && profile.IsLongTerm() && s.serialNumber() != "" {
			return profile
		}

	}

	return nil

}