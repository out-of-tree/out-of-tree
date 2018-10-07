#include <linux/module.h>
#include <linux/kernel.h>

int init_module(void)
{
	return 0;
}

void cleanup_module(void)
{
}

MODULE_LICENSE("GPL");
