package main

func printShellCommandError(command string, err error) {
	display := command
	if len(display) > shellCmdLen {
		display = display[:shellCmdLen] + "..."
	}
	fail("Command \"" + display + "\" errored out (code " + err.Error() + ").")
}

func debugShellCommand(_ string, output []byte, err error) {
	maxOutput := 300
	suffix := "..."
	if len(output) < maxOutput {
		maxOutput = len(output)
		suffix = ""
	}
	if err != nil {
		debug("Command output ( len:", len(output), ") (error:", err.Error()+"):", string(output[:maxOutput])+suffix)
		return
	}
	debug("Command output ( len:", len(output), ") (error: nil):", string(output[:maxOutput])+suffix)
}
