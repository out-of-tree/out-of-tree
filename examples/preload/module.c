#include <linux/module.h>
#include <linux/slab.h>

int init_module(void)
{
	char *argv[] = { "/bin/sh", "--help", NULL };
	char *envp[] = { NULL };

	/* trigger lkrg */
	return call_usermodehelper(argv[0], argv, envp, UMH_WAIT_PROC);
}

void cleanup_module(void)
{
}

MODULE_LICENSE("GPL");
