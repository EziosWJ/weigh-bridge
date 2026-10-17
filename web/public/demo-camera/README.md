# 模拟摄像头图片

这里的图片由 OpenAI 内置 imagegen 工具生成，用于前端称重演示。它们不是现场照片，不代表实际车辆或抓拍证据；图片中的车牌不可读，业务车牌由页面单独展示。

| 文件 | 内容 | 尺寸 |
| --- | --- | --- |
| `truck-front.png` | 蓝白货车停在工厂地磅，左前方固定摄像机视角，日间 | 1672 × 941 |
| `truck-rear.png` | 灰白货车停在工厂地磅，右后方固定摄像机视角，阴天 | 1672 × 941 |

两张图片是独立生成的示例场景，可能出现车型、天气和环境差异，不构成同一车辆的真实多机位记录。演示界面须持续标注“模拟画面／非现场抓拍”。

## 生成提示词

前视：

```text
Use case: photorealistic-natural. Asset type: fictional camera snapshot for a frontend weighbridge demonstration. Create ONE landscape 16:9 ordinary CCTV-style photograph at a generic Chinese grain factory or distillery yard, daylight, looking diagonally from front-left at a blue-and-white cargo truck stopped entirely on a long metal weighbridge platform. Show truck front, side and weighbridge surroundings clearly. Fixed elevated camera, realistic modest surveillance image quality, concrete yard and plain industrial buildings, empty cab without visible people. The license plate must be blank or unreadable. No brand, logo, watermark, timestamps, text, overlays or UI. Fictional scene only.
```

后视：

```text
Use case: photorealistic-natural. Asset type: fictional camera snapshot for a frontend weighbridge demonstration. Create ONE landscape 16:9 ordinary CCTV-style photograph at a generic Chinese grain factory or distillery yard, overcast day, looking diagonally from rear-right at a grey-white cargo truck stopped entirely on a long metal weighbridge platform. Show truck rear, right side and weighbridge platform clearly. Fixed elevated camera, realistic modest surveillance image quality, concrete yard and plain industrial buildings. No visible people or faces. License plate must be blank or unreadable. No branding, logo, watermark, timestamps, text, overlays or UI. Fictional scene only.
```
