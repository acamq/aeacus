#include <stddef.h>
#include <stdio.h>
#include <sys/param.h>
#include <sys/proc.h>
#include <sys/user.h>

#define FIELD(name) printf(#name "\t%zu\t%zu\t%d\n", offsetof(struct kinfo_proc, name), sizeof(((struct kinfo_proc *)0)->name), ((__typeof__(((struct kinfo_proc *)0)->name))-1) < 0)

int main(void) {
    printf("struct_size\t%zu\n", sizeof(struct kinfo_proc));
    printf("p_traced\t%d\n", P_TRACED);
    FIELD(ki_structsize);
    FIELD(ki_pid);
    FIELD(ki_flag);
    FIELD(ki_tracer);
    printf("ki_start\t%zu\t%zu\t0\n", offsetof(struct kinfo_proc, ki_start), sizeof(((struct kinfo_proc *)0)->ki_start));
    return 0;
}
