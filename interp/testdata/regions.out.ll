target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32--wasi"

@value = local_unnamed_addr global ptr @"main$alloc"
@"main$alloc" = internal global [4 x i8] zeroinitializer, align 4

define void @runtime.initAll() unnamed_addr {
  ret void
}
