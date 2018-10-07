#include <linux/module.h>
#include <linux/kernel.h>

int init_module(void)
{
	printk("COOKIE!\n");
	return 0;
}

void cleanup_module(void)
{
}

MODULE_LICENSE("GPL");
