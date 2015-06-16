package containers

import (
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
)

/*
 * General function type for performing something that can
 * return error.
 */
type taskFn func() error

/*
 * General function type for handling errors returned by taskFn
 */
type errorHandlerFn func(err error)

/*
 * Perform a critical section task by locking/unlocking with the
 * given locker/unlocker functions. The task is only executed
 * when the locker function is successfully executed. The wrapper
 * always attempt to unlock using the unlocker function at the
 * end of the task. Error in unlock will override task error if any
 * as the final return value by doExclusive.
 */
func doExclusive(locker taskFn, unlocker taskFn, errHandler errorHandlerFn, task taskFn) (err error) {
	err = locker()
	if err != nil {
		errHandler(err)
		return err
	}

	defer func() {
		unlockErr := unlocker()
		if unlockErr != nil {
			errHandler(unlockErr)
			err = unlockErr
		}
	}()

	err = task()
	if err != nil {
		errHandler(err)
	}

	return err
}

/*
 * Creates a container locker function that locks the given
 * container row in database. Since it returns a task function
 * the actual locking is only done when that function is called.
 */
func lockerFn(cn *container.Container, env *middleware.Env, timeout time.Duration, tempState container.State) taskFn {
	return func() error {
		if cn.LockedUntil != nil && time.Now().After(*cn.LockedUntil) {
			logrus.Warnf("Attempt to lock container with expired lock: %+v", cn)
		}

		if err := cn.Lock(tempState, timeout, env.DB); err != nil {
			if err == container.ErrLocked {
				return errContainerLocked
			}
			return err
		}
		return nil
	}
}

/*
 * Creates a container unlocker function that unlocks the given
 * container row in database. The returned task function will only
 * be called by doExclusive() if the locking is successful.
 */
func unlockerFn(cn *container.Container, env *middleware.Env) taskFn {
	return func() error {
		err := cn.Unlock(env.DB)
		if err != nil {
			logrus.Warnf("Failed to unlock container: %+v: %s", cn, err)
		}

		return err
	}
}

/*
 * Returns a logger function to log errors in locking/unlocking
 * and also errors returned by critical section task.
 */
func errorLoggerFn(cn *container.Container, tempState container.State) errorHandlerFn {
	return func(err error) {
		if err == errContainerLocked {
			logrus.Infof("Failed to lock container: container is already locked. %+v", cn)
		} else {
			logrus.Errorf("Error in container transition state %s for container %+v: %s", tempState, cn, err)
		}
	}
}

/*
 * Do container-specific exclusive task. It accepts required parameters
 * for locking a container, create container-specific locker/unlocker
 * and logger functions and call doExclusive to perform the actual
 * exclusive task.
 */
func doContainerExclusive(cn *container.Container, env *middleware.Env, tempState container.State, lockTime time.Duration, task taskFn) error {
	return doExclusive(
		lockerFn(cn, env, lockTime, tempState),
		unlockerFn(cn, env),
		errorLoggerFn(cn, tempState),
		task)
}

/*
 * Async version of doExclusive. It will return immediately once locking is successful.
 * The critical section task and unlocking is done in a goroutine. Their error cannot
 * be returned and thus can only be handled by given error handler function.
 */
func doAsyncExclusive(locker taskFn, unlocker taskFn, errHandler errorHandlerFn, task taskFn) error {
	err := locker()
	if err != nil {
		errHandler(err)
		return err
	}

	go func() {
		defer func() {
			err := unlocker()
			if err != nil {
				errHandler(err)
			}
		}()

		err := task()
		if err != nil {
			errHandler(err)
		}
	}()

	return nil
}

func doSyncOrAsyncExclusive(async bool, locker taskFn, unlocker taskFn, errHandler errorHandlerFn, task taskFn) error {
	if async {
		return doAsyncExclusive(locker, unlocker, errHandler, task)
	} else {
		return doExclusive(locker, unlocker, errHandler, task)
	}
}

// Async version of doContainerExclusive
func doContainerSyncOrAsyncExclusive(async bool, cn *container.Container, env *middleware.Env, tempState container.State, lockTime time.Duration, task taskFn) error {
	return doSyncOrAsyncExclusive(
		async,
		lockerFn(cn, env, lockTime, tempState),
		unlockerFn(cn, env),
		errorLoggerFn(cn, tempState),
		task)
}
