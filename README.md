
# Kael

Kael 是 JumpServer 连接 GPT 的组件，计划支持多种 GPT, 

Kael 前端 vue3，后端使用 Fastapi 实现， 名字来源 [Kael](https://www.dotafire.com/dota-2/guide/kael-1867)，

作为 Dota 中最强大的法师，能融合多种元素，创造出最多样的魔法，所以取名为 Kael。

## 支持的 GPT

- [x] ChatGPT
- [ ] Bing
- [ ] Bard


## UI 展示

![UI展示](https://download.jumpserver.org/images/kael.png)


## 开发环境

1.下载项目

```shell
$ git clone https://github.com/jumpserver/kael.git
```

2.安装依赖
```shell
$ cd app
$ pip install -r requirements.txt
$ cd ../ui
$ npm install
```
3.运行 API

```shell
$ cd app
$ cp config_example.yml config.yml
$ vim config.yml $ 修改配置文件中的各个 key
$ python main.py
```

4.运行 UI

```shell
$ cd ui
$ cp .env.development .env
$ vim .env $ 修改 VITE_APP_BASE_URL
$ npm run serve
```
