---
name: movie-content
description: 接收用户提供的图片、视频和剧本/脚本，整理成一份可继续制作的内容包。用户提到图片、视频、剧本、脚本、分镜、参考画面、成片片段、screenplay、storyboard，或直接贴了脚本、上传了图片或视频时，都使用这个 skill，哪怕三种输入只给了其中一种。
license: MIT
---

# Movie Content

把图片、视频、脚本收成同一份内容包。三种输入都可以缺，但至少要有一种。不要向用户索要没有的那一种。

## 输入放哪里

只使用沙箱虚拟路径，不要写宿主真实目录。

| 输入 | 虚拟路径 |
| --- | --- |
| 图片 | `/mnt/user-data/uploads/images/` |
| 视频 | `/mnt/user-data/uploads/videos/` |
| 脚本文本 | `/mnt/user-data/workspace/script.md` |
| 汇总清单 | `/mnt/user-data/workspace/intake.json` |
| 内容包 | `/mnt/user-data/outputs/content-brief.md` |

已在 uploads 里的文件不要再复制一遍。用户只在对话里贴了文本或描述时，把文本写入上表对应文件；图片和视频没有文件就在清单里记 `source: "inline"`，不要编造文件路径。

## 步骤

1. 把每条输入分成 `image`、`video`、`script` 之一。分不清就标 `unknown`，并在简报里写明。
2. 脚本写入 `script.md`。保留用户原文，不要改对白和场次编号。
3. 用 `write_file` 写 `intake.json`。图片、视频只记录路径和用户原话，不要把二进制塞进 JSON。
4. 能读的图片用读文件查看内容；视频只记录路径、时长和用户说明，不要整文件读入上下文。
5. 写 `content-brief.md`，然后用一两段话告诉用户收了什么、缺什么、下一步能做什么。

`intake.json` 用这个结构：

```json
{
  "title": "",
  "images": [
    {"path": "/mnt/user-data/uploads/images/ref.png", "note": "", "source": "file"}
  ],
  "videos": [
    {"path": "/mnt/user-data/uploads/videos/clip.mp4", "note": "", "source": "file"}
  ],
  "script": {
    "path": "/mnt/user-data/workspace/script.md",
    "source": "inline"
  }
}
```

没有的数组留空。没有脚本时不要写 `script` 字段。

## 内容包格式

`content-brief.md` 固定用这些标题：

```markdown
# 标题

## 收到的材料
- 图片：
- 视频：
- 脚本：

## 脚本要点
按场次列出地点、人物、动作。没有脚本就写「未提供」。

## 画面与视频对应
每条图片或视频对应到哪一场。对不上就标「未对应」。

## 缺口
还缺的材料。没有就写「无」。
```

不要生成新的图片或视频，除非用户明确要求。这个 skill 只整理已经给到的内容。
