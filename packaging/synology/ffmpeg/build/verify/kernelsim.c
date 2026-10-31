/* kernelsim <names-file> <cmd> [args...]: every syscall named in names-file fails with ENOSYS
 * for cmd and its children, which approximates the syscall surface of an older kernel (SPK-02,
 * packaging/synology/ffmpeg/README.md). The filter works on the build machine's syscalls, so
 * it also covers a QEMU user-mode emulator that binfmt_misc starts for a foreign binary.
 * names-file holds whitespace-separated syscall names only; names unknown to libseccomp are
 * skipped. Runs inside the container, outside Docker's own seccomp profile, because runc
 * itself needs statx and fsopen to start a container.
 * Build: gcc -O2 -static -o kernelsim kernelsim.c -lseccomp (verify/Dockerfile). */
#include <errno.h>
#include <seccomp.h>
#include <stdio.h>
#include <unistd.h>

int main(int argc, char **argv) {
  FILE *f = argc >= 3 ? fopen(argv[1], "r") : NULL;
  if (!f) { fprintf(stderr, "usage: kernelsim names-file cmd [args]\n"); return 2; }
  scmp_filter_ctx ctx = seccomp_init(SCMP_ACT_ALLOW);
  if (!ctx) return 2;
  char name[128];
  while (fscanf(f, "%127s", name) == 1) {
    int nr = seccomp_syscall_resolve_name(name);
    if (nr != __NR_SCMP_ERROR) seccomp_rule_add(ctx, SCMP_ACT_ERRNO(ENOSYS), nr, 0);
  }
  fclose(f);
  if (seccomp_load(ctx) != 0) return 2;
  execvp(argv[2], argv + 2);
  perror("kernelsim: exec");
  return 127;
}
