# 📝 NeonNotes

A minimalist, local-first, floating notes application built with Python and PyWebview. It features a transparent "Widget Mode" and remembers its exact position on your screen.

![Demo GIF](https://github.com/vaelisstudio12/NeonNotes/blob/main/application.gif) 

## ✨ Features

* **Zero Configuration:** Just run `main.py`. The app automatically generates its own HTML, CSS, and JS files on the first run!
* **Widget Mode:** Click a note to shrink the app into a minimal floating widget on your desktop.
* **Smart Positioning:** Remembers your exact window coordinates (both in Main and Widget modes).
* **Customizable Themes:** Change background and text colors directly from the UI.
* **Local & Secure:** All data is saved locally in a thread-safe `data.json` file. No cloud, no tracking.

## 🚀 Installation & Usage

## Warning: 
When changing color, please start your text with #; otherwise, it will be incorrect and the software will not detect it.

## You can customize and change the size of the widget and the home screen.

### 🐧 Linux (Debian/Ubuntu/Mint)
Modern Linux distributions use externally managed environments. Here is the safest way to run the app:

```bash
# 1. Install WebKit dependencies (required for pywebview GUI)
sudo apt install python3-gi gir1.2-webkit2-4.1 python3-venv

# 2. Clone the repository
git clone [https://github.com/vaelisstudio12/NeonNotes](https://github.com/vaelisstudio12/NeonNotes)
cd NeonNotes

# 3. Create virtual environment & install dependencies
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt

# 4. Run the app
python main.py
```

## ☕ Donate
If you like this project, consider buying me a coffee with Monero (XMR):

`45qNiHzBpi83ojK88ppgAS4cQSHRFThqY3JpXaNoQFB8Ap6hK6gFZ64SnTFqajeinjAqff3xjNy918ubRADX53bg2ZDPHUo`
