package main

func startPhocus(loadConfig func() error, runLoop func(func()), shellLauncher func()) error {
	if err := loadConfig(); err != nil {
		return err
	}
	runLoop(shellLauncher)
	return nil
}
