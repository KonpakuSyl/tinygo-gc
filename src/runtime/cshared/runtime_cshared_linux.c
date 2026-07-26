#define _GNU_SOURCE

// The TinyGo sysroots intentionally contain only the headers needed by the Go
// runtime. Keep these ELF and dynamic-loader declarations local instead of
// requiring the full system header set. This file is used by every ELF target
// that supports c-shared: GNU/Linux and Android (bionic).

typedef unsigned short ElfW_Half;
typedef unsigned int ElfW_Word;

#if defined(__LP64__) || defined(_LP64)
typedef unsigned long ElfW_Addr;
typedef unsigned long ElfW_Xword;
typedef unsigned long uintptr_t;
typedef unsigned long size_t;

typedef struct {
    ElfW_Word p_type;
    ElfW_Word p_flags;
    ElfW_Xword p_offset;
    ElfW_Addr p_vaddr;
    ElfW_Addr p_paddr;
    ElfW_Xword p_filesz;
    ElfW_Xword p_memsz;
    ElfW_Xword p_align;
} ElfW_Phdr;
#else
typedef unsigned int ElfW_Addr;
typedef unsigned int ElfW_Xword;
typedef unsigned int uintptr_t;
typedef unsigned int size_t;

// The 32-bit program header keeps its flags after the sizes instead of next to
// the type.
typedef struct {
    ElfW_Word p_type;
    ElfW_Xword p_offset;
    ElfW_Addr p_vaddr;
    ElfW_Addr p_paddr;
    ElfW_Xword p_filesz;
    ElfW_Xword p_memsz;
    ElfW_Word p_flags;
    ElfW_Xword p_align;
} ElfW_Phdr;
#endif

struct dl_phdr_info {
    ElfW_Addr dlpi_addr;
    const char *dlpi_name;
    const ElfW_Phdr *dlpi_phdr;
    ElfW_Half dlpi_phnum;
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

    for (ElfW_Half i = 0; i < info->dlpi_phnum; i++) {
        const ElfW_Phdr *header = &info->dlpi_phdr[i];
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
