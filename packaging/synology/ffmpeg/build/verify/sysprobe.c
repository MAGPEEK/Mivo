/* sysprobe: calls getrandom and statx directly and prints the result of each, so a check run
 * can show that kernelsim's filter is in force (both fail with ENOSYS under the 3.10 list,
 * statx alone under the 4.4 list). getrandom arrived in Linux 3.17, statx in 4.11.
 * Build: gcc -O2 -static -o sysprobe sysprobe.c (verify/Dockerfile). */
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <sys/syscall.h>
#include <unistd.h>

static void report(const char *name, long result) {
  printf("%s=%s\n", name, result >= 0 ? "ok" : (errno == ENOSYS ? "ENOSYS" : strerror(errno)));
}

int main(void) {
  char buffer[16];
  char statx_buffer[256];
  report("getrandom", syscall(SYS_getrandom, buffer, sizeof buffer, 0));
  report("statx", syscall(SYS_statx, AT_FDCWD, "/", 0, 0, statx_buffer));
  return 0;
}
