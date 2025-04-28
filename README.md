<h1 align="center">💦 BPB Wizard</h1>

A wizard to facilitate [BPB Panel](https://github.com/bia-pain-bache/BPB-Worker-Panel) deployments.

<p align="center">
  <img src="assets/wizard.jpg">
</p>
<br>

## 💡 Usage

- You can download executable files from [Releases](https://github.com/bia-pain-bache/BPB-Wizard/releases) based on your OS and just run it.
- Android users (Termux) can use these scripts (Have bug temporarily):

### ARM v8

```bash
curl -L -# -o BPB-Wizard https://github.com/bia-pain-bache/BPB-Wizard/releases/latest/download/BPB-Wizard-linux-arm64 && chmod +x ./BPB-Wizard && ./BPB-Wizard
```

### ARM v7 (Old models)

```bash
curl -L -# -o BPB-Wizard https://github.com/bia-pain-bache/BPB-Wizard/releases/latest/download/BPB-Wizard-linux-arm && chmod +x ./BPB-Wizard && ./BPB-Wizard
```

> [!TIP]
> 1- First it logs you in your Cloudflare account.
>
> 2- Wizard will ask some questions for setting Panel and Configs secrets. All secrets are generated safely and randomly. However, you can use default generated values or just enter desired values.
>
> 3- Opens your Panel in browser! Enjoy it...
