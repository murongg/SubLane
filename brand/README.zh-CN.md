# SubLane 品牌资源

[English](README.md)

仓库只保留日常使用的资源和生成源：

- `logos/lockup-black.svg`、`lockup-white.svg`：中英文 README 使用的横向 Logo。
- `social/github-dark.svg`、`github-dark.png`：选定的 GitHub 社交预览图。
- `source/Inter.ttf`、`OFL.txt`：用于重新导出的字体及许可。
- `tokens.json`：色值、字体和留白规范。

原始标志保存在 [`docs/assets/logo.svg`](../docs/assets/logo.svg)，生成脚本只维护 `tools/brand/` 中的一份。

## 使用规则

保持标志比例与两条路径不变。浅底使用黑色 Logo，深底使用白色 Logo；常规排版四周保留至少四分之一画布宽度的空白。日常界面建议使用 24px 以上图形，小尺寸浏览器图标使用专门导出的 favicon。

主体保持黑白灰，彩色只用于状态。字标使用转曲后的 Inter 650，不改变应用界面的字体。品牌资源采用项目的 [AGPL-3.0-only 协议](../LICENSE)，字体保留独立的 [SIL OFL 1.1 许可](source/OFL.txt)。

## 完整物料

在项目根目录运行：

```sh
make brand
```

完整物料输出到 **`dist/brand/`**，已被 Git 忽略；命令只会同步更新仓库内的横向 Logo、选定的社交预览图和色值文件。

打开 `dist/brand/preview.html` 可浏览头像、图标、PNG 变体、横幅、宣传图、演示封面、壁纸和贴纸。使用指南与独立生成器也包含在导出目录中。

完整 ZIP 适合单独下载或作为 Release 附件，不再把所有导出提交进仓库。生成命令不会发布 Release。
