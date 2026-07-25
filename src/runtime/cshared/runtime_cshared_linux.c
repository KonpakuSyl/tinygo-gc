#define _GNU_SOURCE

// The TinyGo glibc sysroot intentionally contains only the headers needed by
// the Go runtime. Keep these ELF and dynamic-loader declarations local instead
// of requiring the full host glibc header set.
typedef unsigned short Elf64_Half;
typedef unsigned int Elf64_Word;
typedef unsigned long Elf64_Addr;
typedef unsigned long Elf64_Xword;
typedef unsigned long uintptr_t;
typedef unsigned long size_t;

typedef struct {
    Elf64_Word p_type;
    Elf64_Word p_flags;
    Elf64_Xword p_offset;
    Elf64_Addr p_vaddr;
    Elf64_Addr p_paddr;
    Elf64_Xword p_filesz;
    Elf64_Xword p_memsz;
    Elf64_Xword p_align;
} Elf64_Phdr;

struct dl_phdr_info {
    Elf64_Addr dlpi_addr;
    const char *dlpi_name;
    const Elf64_Phdr *dlpi_phdr;
    Elf64_Half dlpi_phnum;
};

typedef struct {
    const char *dli_fname;
    void *dli_fbase;
    const char *dli_sname;
    void *dli_saddr;
} Dl_info;

extern int dladdr(const void *address, Dl_info *info);
extern int dl_iterate_phdr(int (*callback)(struct dl_phdr_info *, size_t, void *), void *data);

#define PT_LOAD 1
#define PF_W 2

extern void tinygo_cshared_global_segment(uintptr_t start, uintptr_t end);

static uintptr_t tinygo_cshared_base;

static int tinygo_cshared_find_globals_callback(struct dl_phdr_info *info,
                                                 size_t size, void *data) {
    (void)size;
    (void)data;

    if ((uintptr_t)info->dlpi_addr != tinygo_cshared_base) {
        return 0;
    }

    for (Elf64_Half i = 0; i < info->dlpi_phnum; i++) {
        const Elf64_Phdr *header = &info->dlpi_phdr[i];
        if (header->p_type != PT_LOAD || !(header->p_flags & PF_W)) {
            continue;
        }
        uintptr_t start = (uintptr_t)info->dlpi_addr + header->p_vaddr;
        tinygo_cshared_global_segment(start, start + header->p_memsz);
    }
    return 1;
}

void tinygo_cshared_find_globals(void) {
    Dl_info info;
    if (dladdr((void *)&tinygo_cshared_find_globals, &info) == 0) {
        return;
    }
    tinygo_cshared_base = (uintptr_t)info.dli_fbase;
    dl_iterate_phdr(tinygo_cshared_find_globals_callback, 0);
}
