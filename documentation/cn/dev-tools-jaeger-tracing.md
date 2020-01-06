# Jaeger跟踪

Lotus 在其很多内部构件中都构建了追踪。要查看这些跟踪，先下载 [Jaeger](https://www.jaegertracing.io/download/)（选择“all-in-one”二进制），然后运行，启动lotus守护进程，在浏览器中打开localhost:16686。

## Open Census

Lotus用 [OpenCensus](https://opencensus.io/)来跟踪应用流。这会通过执行带注释的代码路径生成跨度。

目前它会使用Jaeger，尽管其他跟踪后端应该相当容易换入。

## 本地运行

为了简单地在本地运行和查看跟踪，首先安装jaeger，最简单的方法是[download the binaries](https://www.jaegertracing.io/download/) 然后运行`jaeger-all-in-one` 二进制。这将会启动jaeger, 在`localhost:6831`监听跨度，并在`http://localhost:16686/`提供一个网页UI以供查看跟踪。

现在开始，将跟踪从Lotus发到Jaeger, 设置环境`LOTUS_JAEGER` to `localhost:6831`, 并在浏览器中启动`lotus daemon`.

## 添加跨度

要使用跨度注释一个新的代码路径，请在希望跟踪的函数的顶部添加以下行：

```go
ctx, span := trace.StartSpan(ctx, "put function name here")
defer span.End()
```
