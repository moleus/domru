package auth

type CredentialsStore interface {
	SaveCredentials(credentials AuthenticationResponse) error
	LoadCredentials() (AuthenticationResponse, error)
}
