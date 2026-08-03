target datalayout = "e-m:e-p:32:32-i64:64-n32:64-S128"
target triple = "wasm32--wasi"

@value = global ptr null

declare void @runtime.regionEnter(ptr) unnamed_addr
declare void @runtime.regionExit(ptr) unnamed_addr
declare ptr @runtime.regionAlloc(ptr, i32, ptr) unnamed_addr

define void @runtime.initAll() unnamed_addr {
  call void @main.init()
  ret void
}

define internal void @main.init() unnamed_addr {
  %owner = alloca i8
  call void @runtime.regionEnter(ptr %owner)
  %value = call ptr @runtime.regionAlloc(ptr %owner, i32 4, ptr null)
  store ptr %value, ptr @value
  call void @runtime.regionExit(ptr %owner)
  ret void
}
