package controller

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/consensys/linea-monorepo/prover/cmd/controller/controller/metrics"
	"github.com/consensys/linea-monorepo/prover/config"
	"github.com/consensys/linea-monorepo/prover/utils"
	"github.com/sirupsen/logrus"
)

// List of the possible errors returned by the prover as an exit code. There are
// other possibilities but the one listed here are the one of interests. Some of
// these codes are generated by the executor and not the child process itself.
// These includes, CodeFatal, CodeTooManyRetries and CodeCantRunCommand.
const (
	CodeSuccess        int = 0   // Success code
	CodeTraceLimit     int = 77  // The traces are overflown
	CodeOom            int = 137 // When the process exits on OOM
	CodeFatal          int = 14  // When the process could not start
	CodeCantRunCommand int = 15  // When the controller could not run the command
)

// Status of a finished job
type Status struct {
	// The matched return code
	ExitCode int
	// String explaining what when wrong. "success"
	What string
	// Additional errors for context
	Err error
}

// Resource collects all the informations about the job that can be used to
// fill the input command template. The names of the fields are what defines the
// API of the command templating.
type Resource struct {
	ConfFile string
	// The input and output file paths
	InFile, OutFile string
}

// The executor is responsible for running the commands specified by the jobs
type Executor struct {
	Config *config.Config
	// Logger specific to the executor
	Logger *logrus.Entry
}

func NewExecutor(cfg *config.Config) *Executor {
	return &Executor{
		Config: cfg,
		Logger: cfg.Logger().WithField("component", "executor"),
	}
}

// Run an execution proof job. Importantly, this code must NOT panic because no
// matter what happens we want to be able to gracefully shutdown.
func (e *Executor) Run(job *Job) (status Status) {

	// The job should be locked
	if len(job.LockedFile) == 0 {
		return Status{
			ExitCode: CodeFatal,
			What:     "the job is not locked",
		}
	}

	// Determine if it's a large job based on the file suffix and config
	// Note: checking that locked job contains "large" is not super typesafe...
	largeRun := job.Def.Name == jobNameExecution && e.Config.Execution.CanRunFullLarge && strings.Contains(job.LockedFile, config.LargeSuffix)

	// Build the initial command (normal for regular jobs, large for large jobs)
	cmd, err := e.buildCmd(job, largeRun)
	if err != nil {
		return Status{
			ExitCode: CodeCantRunCommand,
			Err:      err,
			What:     "can't format the command",
		}
	}

	// Run the initial command
	status = runCmd(cmd, job, false)

	// Do not retry for blob decompression or aggregation jobs
	if job.Def.Name == jobNameBlobDecompression || job.Def.Name == jobNameAggregation {
		return status
	}

	// If the initial run succeeds, return the status
	if status.ExitCode == CodeSuccess {
		return status
	}

	// Check if the exit code is retryable
	retryableCodes := e.Config.Controller.RetryLocallyWithLargeCodes
	if isIn(status.ExitCode, retryableCodes) {
		if largeRun {
			// For large jobs, retry with the same large command
			status = runCmd(cmd, job, true)
		} else {
			// For regular jobs, retry with the large command
			largeCmd, err := e.buildCmd(job, true)
			if err != nil {
				return Status{
					ExitCode: CodeCantRunCommand,
					Err:      err,
					What:     "can't format the command",
				}
			}
			status = runCmd(largeCmd, job, true)
		}
	}

	return status
}

// Builds a command from a template to run, returns a status if it failed
func (e *Executor) buildCmd(job *Job, large bool) (cmd string, err error) {

	// The generates a name for the output file. Also attempts to generate the
	// name of the final response file so that we can be sure it will be
	// not fail being generated after having run the command.
	if _, err := job.ResponseFile(); err != nil {
		logrus.Errorf(
			"could not generate the tmp response filename for %s: %v",
			job.OriginalFile, err,
		)
		return "", err
	}
	outFile := job.TmpResponseFile(e.Config)

	tmpl := e.Config.Controller.WorkerCmdTmpl
	if large {
		tmpl = e.Config.Controller.WorkerCmdLargeTmpl
	}

	// use the template to generate the command
	resource := Resource{
		ConfFile: fConfig,
		InFile:   job.InProgressPath(),
		OutFile:  outFile,
	}

	// Build the command and args from the job
	w := &strings.Builder{}
	if err := tmpl.Execute(w, resource); err != nil {
		logrus.Errorf(
			"tried to generate the command for job %s but got %v",
			job.OriginalFile, err,
		)

		// Returns a status indicating that the command templating failed
		return "", err
	}

	// Successfully built the command
	return w.String(), nil
}

// Run a command and returns the status. Retry gives an indication on whether
// this is a local retry or not.
func runCmd(cmd string, job *Job, retry bool) Status {

	// Split the command into a list of argvs that can be passed to the os
	// package.
	logrus.Infof("The executor is about to run the command: %s", cmd)

	// The command is run through shell, that way it sparses us the requirement
	// to tokenize the quoted string if the command contains any.
	var err error
	argvs := []string{"sh", "-c", cmd}
	argvs[0], err = exec.LookPath(argvs[0])
	if err != nil {
		return Status{
			ExitCode: CodeCantRunCommand,
			Err:      err,
			What: fmt.Sprintf(
				"Could not find `%v` on the system. We need it to run the command",
				argvs[0],
			),
		}
	}

	pname := processName(job, cmd)

	metrics.CollectPreProcess(job.Def.Name, job.Start, job.End, false)

	// Starts a new process from our command
	startTime := time.Now()
	curProcess, err := os.StartProcess(
		argvs[0], argvs,
		// Pipe the child process's stdin/stdout/stderr into the current process
		&os.ProcAttr{
			Files: []*os.File{
				os.Stdin,
				os.Stdout,
				os.Stderr,
			},
		},
	)

	if err != nil {
		// Failing to start the process can happen for various different
		// reasons. It can be that the commands contains invalid characters
		// like "\0". In practice, some of theses errors might be retryable
		// and remains to see which one can. Until then, they will need to
		// be manually retried.
		logrus.Errorf("unexpected : failed to start process %v with error", pname)
		return Status{
			ExitCode: CodeFatal,
			What:     "got an error starting the process",
			Err:      err,
		}
	}

	// Lock on the process until it finishes
	pstate, err := curProcess.Wait()
	if err != nil {
		// Here it means, the "os" package could not start the process. It
		// can happen for many different reasons essentially pertaining to
		// the initialization of the process. It may be that some of theses
		// errors are retryables but it remains to see which one. Until then
		// we exited with a fatal code and the files will need to be
		// manually reprocessed.
		logrus.Errorf("unexpected : got an error trying to lock on %v : %v", pname, err)
		return Status{
			ExitCode: CodeFatal,
			What:     "got an error waiting for the process",
			Err:      err,
		}
	}

	processingTime := time.Since(startTime)
	exitCode, err := unixExitCode(pstate)
	if err != nil {
		// NB: this would be only possible if we did not wait for the process
		// to finish trying to read the exit code.
		utils.Panic("unexpectedly got an error while trying to read the exit code of the process: %v", err)
	}

	logrus.Infof(
		"The processing of file `%s` (process=%v) took %v seconds to complete and returned exit code %v",
		job.OriginalFile, pname, processingTime.Seconds(), exitCode,
	)

	// Build the  response status
	status := Status{ExitCode: exitCode}
	switch status.ExitCode {
	case CodeSuccess:
		status.What = "success"
	case CodeOom:
		status.What = "out of memory error"
	case CodeTraceLimit:
		status.What = "trace limit overflow"
	}

	metrics.CollectPostProcess(job.Def.Name, status.ExitCode, processingTime, retry)

	return status
}

// Returns a human-readable process name. The process name is formatted as in
// the following example: `execution-102-103-<unix-timestamp> <command>`. The
// uuid at the end is there to ensure that two processes never share the same
// name.
func processName(job *Job, cmd string) string {
	return fmt.Sprintf(
		"%v-%v-%v-%v %v",
		job.Def.Name, job.Start, job.End, time.Now().UTC().Unix(), cmd,
	)
}

// Returns true if the x is included in the given list
func isIn[T comparable](x T, list []T) bool {
	for _, y := range list {
		if x == y {
			return true
		}
	}
	return false
}

// Returns the exit code of a process when it exited. The reason for this
// function is that gol std library ExitCode() returns -1 when the process has
// been terminated by a signal. This function prevents that behaviour and
// returns the UNIX exit code. The function returns an error if the process is
// still running. Note that this function will panic on non-UNIX platforms.
func unixExitCode(proc *os.ProcessState) (int, error) {

	waitStatus, ok := proc.Sys().(syscall.WaitStatus)
	if !ok {
		utils.Panic("The controller assumes the underlying process to be UNIX and cannot function properly with the current OS.")
	}

	// Note: here we trust the underlying implementation that if "Exited()",
	// then "ExitStatus()" will return a non-negative value
	if waitStatus.Exited() {
		exitcode := waitStatus.ExitStatus()
		return exitcode, nil
	}

	// Note: "CoreDump" is, in principle, a sub-case of signal receival. But we
	// add a second check just for ease of mind. We trust that the signal is
	// non-negative here.
	if waitStatus.Signaled() || waitStatus.CoreDump() {
		sigCode := waitStatus.Signal()
		return 128 + int(sigCode), nil
	}

	return -1, fmt.Errorf("getting the unix exit code : the process has an unexpected status : %v, it should be terminated", proc.String())
}
