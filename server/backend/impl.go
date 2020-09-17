package backend

var ImplMaxPostSize = func(b *Backend) int64 {
	return 1024 * 1024
}

var ImplAuth = func(b *Backend, user, pass string) error {
	return nil
}

var ImplIsMod = func(b *Backend) bool {
	return false
}
