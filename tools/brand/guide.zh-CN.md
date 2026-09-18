# SubLane 品牌物料

[English](README.md)

整套物料使用你选定的最初版标志：两段错位通道组成 S。所有版本保留同样的两条 SVG 路径。

在浏览器中打开 `preview.html` 可以分类浏览并下载；`overview.png` 是物料总览。

## 常用文件

- **Logo**：`logos/`，含图形、横版、竖版和纯字标，均提供黑白两版。
- **头像和应用图标**：`icons/avatar-*`、`icons/app-*`，1024px。
- **浏览器图标**：`icons/favicon.svg`、`favicon.ico`，以及 16–256px PNG。
- **README 头图**：`banners/readme-*`，1600 × 420。
- **项目横幅**：`banners/profile-*`，1500 × 500。
- **GitHub 社交预览**：`social/github-*`，1280 × 640。
- **链接分享卡片**：`social/opengraph-*`，1200 × 630。
- **方形宣传图**：`social/announcement-*`，1080 × 1080，文案明确处于基础框架阶段。
- **演示封面**：`presentation/cover-*`，1920 × 1080。
- **桌面壁纸**：`wallpapers/desktop-*`，3840 × 2160。
- **圆形贴纸图稿**：`stickers/round-*`。
- **规范与色值**：`guidelines/clear-space.svg`、`tokens.json`。

`black/white` 指透明底 Logo 的颜色；`light/dark` 指整张物料的底色方案。SVG 可无损缩放，PNG 可直接上传，所有字标和宣传文案都已转为路径，不依赖本机字体。

## 使用约定

保持标志比例，不拉伸、不旋转、不加渐变。常规排版给标志四周留出至少四分之一画布宽度的空白。日常界面建议使用 24px 以上图形；浏览器 16px 场景使用随附的 favicon。

界面主体保持黑白灰，绿色、琥珀色、红色和蓝色只表达状态。字标使用 Inter 650，应用界面的字体保持原有设置。

中英文项目 README 使用透明底横向 Logo 组合，配合许可证、技术栈和项目阶段徽章；整张头图仍保留为可选物料。GitHub 预览图需要在仓库 Settings → Social preview 中手动上传，生成文件不会修改远端设置。应用 favicon 的接入示例见英文使用指南。

贴纸文件是图稿，正式印刷前还需按印厂要求处理出血、刀线和色彩配置。

## 再次导出

在项目根目录运行 `make brand`，完整物料会导出到 Git 忽略的 `dist/brand/`。源码目录仅保留常用资源和生成源；生成器位于 `tools/brand/`，其依赖与应用运行时隔离。

下载包内也包含独立生成器。在 `source/generator/` 中依次运行 `pnpm install --frozen-lockfile`、`pnpm build` 和 `pnpm check`，无需应用仓库即可再次导出。需要 Node.js 22.12+ 和 pnpm 9.12.2。
