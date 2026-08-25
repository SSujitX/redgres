package postgresadmin

func rotationEligible(owner string, protected, ownerCanLogin, ownerIsSuperuser bool) bool {
	return owner != "" && !protected && ownerCanLogin && !ownerIsSuperuser
}
