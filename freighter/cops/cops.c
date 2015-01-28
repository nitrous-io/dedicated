// formated with indent -linux nsenter.c
#include <errno.h>
#include <fcntl.h>
#include <linux/limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define pr_perror(fmt, ...) fprintf(stderr, "nsenter: " fmt ": %m\n", ##__VA_ARGS__)

static const kBufSize = 256;
static const char *kCops = "cops-";

void get_args(int *argc, char ***argv)
{
	// Read argv
	int fd = open("/proc/self/cmdline", O_RDONLY);
	if (fd < 0) {
		pr_perror("Unable to open /proc/self/cmdline");
		exit(1);
	}
	// Read the whole commandline.
	ssize_t contents_size = 0;
	ssize_t contents_offset = 0;
	char *contents = NULL;
	ssize_t bytes_read = 0;
	do {
		contents_size += kBufSize;
		contents = (char *)realloc(contents, contents_size);
		bytes_read =
		    read(fd, contents + contents_offset,
			 contents_size - contents_offset);
		if (bytes_read < 0) {
			pr_perror("Unable to read from /proc/self/cmdline");
			exit(1);
		}
		contents_offset += bytes_read;
	}
	while (bytes_read > 0);
	close(fd);

	// Parse the commandline into an argv. /proc/self/cmdline has \0 delimited args.
	ssize_t i;
	*argc = 0;
	for (i = 0; i < contents_offset; i++) {
		if (contents[i] == '\0') {
			(*argc)++;
		}
	}
	*argv = (char **)malloc(sizeof(char *) * ((*argc) + 1));
	int idx;
	for (idx = 0; idx < (*argc); idx++) {
		(*argv)[idx] = contents;
		contents += strlen(contents) + 1;
	}
	(*argv)[*argc] = NULL;
}

// Use raw setns syscall for versions of glibc that don't include it (namely glibc-2.12)
#if __GLIBC__ == 2 && __GLIBC_MINOR__ < 14
#define _GNU_SOURCE
#include <sched.h>
#include "syscall.h"
#ifdef SYS_setns
int setns(int fd, int nstype)
{
	return syscall(SYS_setns, fd, nstype);
}
#endif
#endif

void cops_start()
{
	int argc, c;
	char **argv;
	get_args(&argc, &argv);

	// check argv 0 to ensure that we are supposed to setns as we only want to
	// setns for cops-* processes
	if (strncmp(argv[0], kCops, strlen(kCops)) != 0) {
		return;
	}

	pid_t init_pid = -1;
	char *init_pid_str = argv[1];
	if (init_pid_str == NULL) {
		pr_perror("Missing init pid");
		exit(1);
	}

	init_pid = strtol(argv[1], NULL, 10);
	if ((init_pid == 0 && errno == EINVAL) || errno == ERANGE) {
		pr_perror("Failed to parse PID from \"%s\" with output \"%d\"",
			  init_pid_str, init_pid);
		exit(1);
	}
	// Setns on mnt namespace.
	char mnt_ns[PATH_MAX];
	memset(mnt_ns, 0, PATH_MAX);
	snprintf(mnt_ns, PATH_MAX - 1, "/proc/%d/ns/mnt", init_pid);

	int fd;
	fd = open(mnt_ns, O_RDONLY);
	if (fd < 0) {
		pr_perror("Unable to open %s", mnt_ns);
		exit(1);
	}
	// Set the namespace.
	if (setns(fd, 0) == -1) {
		pr_perror("Failed to setns for mnt namespace");
		exit(1);
	}
	close(fd);

	// Finish executing, let the Go runtime take over.
	return;
}
