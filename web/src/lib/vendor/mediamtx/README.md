# MediaMTX WebRTC reader

来源：MediaMTX **v1.21.2**，与当前摄像头模拟服务器版本一致。

- https://github.com/bluenviron/mediamtx/blob/v1.21.2/internal/servers/webrtc/reader.js
- MIT 许可见同目录 `LICENSE`，构建时一并输出第三方许可。

本地改动：改为 ES module 默认导出；关闭时取消未完成的 HTTP 请求、删除 WHEP 会话并清理引用；会话删除失败不会产生未处理的 Promise rejection；PATCH／DELETE 沿用 reader 的授权头；关闭后忽略 track 回调。播放器在外层处理超时、冻结及退避重连，不使用 CDN。
