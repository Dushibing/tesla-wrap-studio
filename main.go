package main

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ---------------- 配置 ----------------

const Port = ":12345"

//go:embed templates
var templateFS embed.FS // 打包资源

// ---------------- 数据结构 ----------------

type CarModel struct {
	ID   string
	Name string
}

var supportedModels []CarModel

type Point struct {
	X, Y int
}

// ---------------- 主程序 ----------------

func main() {
	initModels() // 初始化模版

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/process", handleProcess)
	http.Handle("/templates/", http.FileServer(http.FS(templateFS)))

	url := "http://localhost" + Port
	fmt.Printf("\n--------------------------------------------------\n")
	fmt.Printf("   Tesla Wrap Generator (Final Version)\n")
	fmt.Printf("   服务地址: %s\n", url)
	fmt.Printf("   请保持此窗口开启。\n")
	fmt.Printf("--------------------------------------------------\n\n")

	go openBrowser(url)

	if err := http.ListenAndServe(Port, nil); err != nil {
		log.Fatal("启动失败: ", err)
	}
}

// ---------------- 工具函数 ----------------

func openBrowser(url string) {
	time.Sleep(500 * time.Millisecond)
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		log.Printf("无法自动打开浏览器，请访问: %s\n", url)
	}
}

func initModels() {
	files, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		log.Fatal("❌ 严重错误：未找到嵌入的 templates 资源。\n", err)
	}
	for _, f := range files {
		if !f.IsDir() && (strings.HasSuffix(strings.ToLower(f.Name()), ".png")) {
			id := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
			supportedModels = append(supportedModels, CarModel{ID: id, Name: strings.ToUpper(id)})
			log.Printf("✅ 已加载: %s", id)
		}
	}
	if len(supportedModels) == 0 {
		log.Fatal("❌ templates 文件夹为空！")
	}
}

// ---------------- HTTP Handlers ----------------

func handleHome(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("index").Parse(htmlPage)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	data := struct{ Models []CarModel }{Models: supportedModels}
	tmpl.Execute(w, data)
}

func handleProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	// 允许最大 50MB 上传
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, "Error: "+err.Error(), 400)
		return
	}

	base64Str := r.FormValue("image_data")
	modelID := r.FormValue("model_id")

	// 1. 解码前端图片
	idx := strings.Index(base64Str, ",")
	if idx < 0 {
		http.Error(w, "Invalid image data", 400)
		return
	}
	reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(base64Str[idx+1:]))
	userLayoutImg, _, err := image.Decode(reader)
	if err != nil {
		http.Error(w, "Decode failed", 400)
		return
	}

	// 2. 读取模版
	tmplPath := "templates/" + modelID + ".png"
	tmplFile, err := templateFS.Open(tmplPath)
	if err != nil {
		http.Error(w, "Template not found", 500)
		return
	}
	defer tmplFile.Close()
	targetTemplateImg, _, err := image.Decode(tmplFile)

	// 3. 处理
	bounds := userLayoutImg.Bounds()
	filledImg := image.NewNRGBA(bounds)
	draw.Draw(filledImg, bounds, userLayoutImg, image.Point{}, draw.Src)

	smartFillBFS(filledImg)
	finalImg := applyTemplateMask(filledImg, targetTemplateImg)

	// 4. 返回流 (这里不设置 attachment，方便前端处理 blob)
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, finalImg)
}

// ---------------- 核心算法 ----------------

func smartFillBFS(img *image.NRGBA) {
	bounds := img.Bounds()
	w, h := bounds.Max.X, bounds.Max.Y
	visited := make([]bool, w*h)
	queue := make([]Point, 0, w*h/10)
	hasSeeds := false
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if img.Pix[img.PixOffset(x, y)+3] > 10 {
				queue = append(queue, Point{x, y})
				visited[y*w+x] = true
				hasSeeds = true
			}
		}
	}
	if !hasSeeds {
		draw.Draw(img, bounds, &image.Uniform{color.RGBA{100, 100, 100, 255}}, image.Point{}, draw.Src)
		return
	}
	dirs := []Point{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		srcOff := img.PixOffset(p.X, p.Y)
		r, g, b, a := img.Pix[srcOff], img.Pix[srcOff+1], img.Pix[srcOff+2], img.Pix[srcOff+3]
		for _, d := range dirs {
			nx, ny := p.X+d.X, p.Y+d.Y
			if nx >= 0 && nx < w && ny >= 0 && ny < h {
				nIdx := ny*w + nx
				if !visited[nIdx] {
					dstOff := img.PixOffset(nx, ny)
					img.Pix[dstOff], img.Pix[dstOff+1], img.Pix[dstOff+2], img.Pix[dstOff+3] = r, g, b, a
					visited[nIdx] = true
					queue = append(queue, Point{nx, ny})
				}
			}
		}
	}
}

func applyTemplateMask(bg image.Image, mask image.Image) image.Image {
	bounds := bg.Bounds()
	out := image.NewRGBA(bounds)
	draw.Draw(out, bounds, bg, image.Point{}, draw.Src)
	maskBounds := mask.Bounds()
	var wg sync.WaitGroup
	workers := 8
	chunkH := bounds.Max.Y / workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(startY, endY int) {
			defer wg.Done()
			for y := startY; y < endY; y++ {
				for x := 0; x < bounds.Max.X; x++ {
					if x < maskBounds.Max.X && y < maskBounds.Max.Y {
						mc := mask.At(x, y)
						mr, mg, mb, _ := mc.RGBA()
						if mr > 61600 && mg > 61600 && mb > 61600 { continue }
						out.Set(x, y, mc)
					}
				}
			}
		}(i*chunkH, (i+1)*chunkH)
	}
	wg.Wait()
	return out
}

// ---------------- 前端页面 (含预览 Modal) ----------------

const htmlPage = `
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <title>Tesla 智能车衣设计器 (Preview Mode)</title>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/fabric.js/5.3.1/fabric.min.js"></script>
    <style>
        * { box-sizing: border-box; }
        body { font-family: 'Segoe UI', sans-serif; background: #1e1e1e; color: #eee; margin: 0; display: flex; height: 100vh; overflow: hidden; }
        
        /* 侧边栏 */
        .sidebar { width: 320px; background: #2d2d2d; padding: 20px; border-right: 1px solid #444; display: flex; flex-direction: column; z-index: 10; overflow-y: auto;}
        .main { flex: 1; display: flex; align-items: center; justify-content: center; background: #111; }
        
        h2 { margin-top: 0; color: #fff; font-size: 20px; }
        h2 span { color: #e82127; }
        
        .control-section { margin-bottom: 20px; padding-bottom: 15px; border-bottom: 1px solid #444; }
        label { display: block; margin-bottom: 8px; color: #bbb; font-size: 14px; }
        select, input[type="text"] { width: 100%; padding: 10px; background: #3d3d3d; border: 1px solid #555; color: white; border-radius: 4px; outline: none; }
        select:focus, input[type="text"]:focus { border-color: #e82127; }

        .upload-btn-wrap { position: relative; width: 100%; background: #3d3d3d; border: 1px dashed #666; border-radius: 6px; padding: 10px; margin-bottom: 10px; cursor: pointer; transition: 0.2s; }
        .upload-btn-wrap:hover { background: #444; border-color: #999; }
        .upload-btn-wrap input { position: absolute; left: 0; top: 0; opacity: 0; width: 100%; height: 100%; cursor: pointer; }
        .upload-text { display: flex; justify-content: space-between; font-size: 13px; color: #ccc; }

        .btn-gen { background: linear-gradient(135deg, #e82127 0%, #b30000 100%); color: white; border: none; padding: 15px; width: 100%; font-size: 16px; font-weight: bold; cursor: pointer; border-radius: 6px; margin-top: auto; transition: 0.2s; }
        .btn-gen:hover { transform: translateY(-2px); box-shadow: 0 5px 15px rgba(232,33,39,0.4); }
        .btn-gen:disabled { background: #555; transform: none; box-shadow: none; cursor: not-allowed; }

        .canvas-frame { border: 2px solid #444; box-shadow: 0 0 50px rgba(0,0,0,0.8); background: #000; }
        
        /* Loading Overlay */
        #loading-overlay { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.8); z-index: 2000; align-items: center; justify-content: center; flex-direction: column; }
        .spinner { border: 4px solid #333; border-top: 4px solid #e82127; border-radius: 50%; width: 40px; height: 40px; animation: spin 1s linear infinite; margin-bottom: 20px; }
        @keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }

        /* Preview Modal */
        #preview-modal { display: none; position: fixed; top: 0; left: 0; width: 100%; height: 100%; background: rgba(0,0,0,0.95); z-index: 3000; align-items: center; justify-content: center; flex-direction: column; }
        #preview-img { max-height: 75vh; max-width: 90vw; border: 1px solid #555; box-shadow: 0 0 30px #000; margin-bottom: 20px; }
        .modal-actions { display: flex; gap: 20px; }
        .btn-action { padding: 12px 30px; font-size: 16px; border: none; border-radius: 5px; cursor: pointer; font-weight: bold; transition: 0.2s; }
        .btn-cancel { background: #444; color: #fff; }
        .btn-cancel:hover { background: #555; }
        .btn-save { background: #e82127; color: #fff; }
        .btn-save:hover { background: #ff3333; box-shadow: 0 0 15px rgba(232,33,39,0.5); }
    </style>
</head>
<body>

<div class="sidebar">
    <h2><span>TESLA</span> Wrap Studio</h2>
    
    <div class="control-section">
        <label>1. 车型 (Model)</label>
        <select id="model-select" onchange="changeModel()">
            {{range .Models}}
            <option value="{{.ID}}">{{.Name}}</option>
            {{end}}
        </select>
    </div>

    <div class="control-section">
        <label>2. 贴图素材 (Upload)</label>
        <div class="upload-btn-wrap"><input type="file" id="file-hood"><div class="upload-text"><span>车头 / Hood</span></div></div>
        <div class="upload-btn-wrap"><input type="file" id="file-left"><div class="upload-text"><span>左侧 / Left</span></div></div>
        <div class="upload-btn-wrap"><input type="file" id="file-right"><div class="upload-text"><span>右侧 / Right</span></div></div>
        <div class="upload-btn-wrap"><input type="file" id="file-rear"><div class="upload-text"><span>车尾 / Rear</span></div></div>
    </div>

    <div class="control-section">
        <label>3. 导出设置 (Filename)</label>
        <input type="text" id="file-name" placeholder="tesla_design_v1" value="my_tesla_design">
    </div>

    <button class="btn-gen" onclick="generatePreview()">生成预览 (Generate Preview)</button>
</div>

<div class="main">
    <div class="canvas-frame">
        <div class="canvas-container-inner"><canvas id="c" width="1024" height="1024"></canvas></div>
    </div>
</div>

<div id="loading-overlay">
    <div class="spinner"></div>
    <div style="color:white; font-weight:bold;">正在渲染高清大图...</div>
</div>

<div id="preview-modal">
    <h3 style="color:white; margin-top:0;">生成结果预览</h3>
    <img id="preview-img" src="">
    <div class="modal-actions">
        <button class="btn-action btn-cancel" onclick="closePreview()">返回修改 (Back)</button>
        <button class="btn-action btn-save" onclick="downloadImage()">确认保存 (Save .PNG)</button>
    </div>
</div>

<script>
    const canvas = new fabric.Canvas('c');
    let currentBlobURL = null; // 存储生成的图片地址

    // 自动缩放
    function resizeCanvas() {
        const h = document.querySelector('.main').clientHeight * 0.85;
        const inner = document.querySelector('.canvas-container-inner') || document.querySelector('.canvas-container');
        if(inner) {
            inner.style.width = h + 'px'; inner.style.height = h + 'px';
            const scale = h / 1024; canvas.setZoom(scale); canvas.setWidth(h); canvas.setHeight(h);
        }
    }
    window.addEventListener('resize', resizeCanvas);
    setTimeout(resizeCanvas, 100);

    function changeModel() {
        const id = document.getElementById('model-select').value;
        canvas.setBackgroundImage(null, canvas.renderAll.bind(canvas));
        fabric.Image.fromURL('/templates/' + id + '.png', function(img) {
            if(!img) return;
            canvas.setBackgroundImage(img, canvas.renderAll.bind(canvas), { opacity: 0.4, originX: 'left', originY: 'top' });
        });
    }

    function bindUpload(id, x, y) {
        document.getElementById(id).addEventListener('change', function(e) {
            const f = e.target.files[0];
            if(!f) return;
            const r = new FileReader();
            r.onload = function(d) {
                fabric.Image.fromURL(d.target.result, function(img) {
                    const s = 300 / Math.max(img.width, img.height);
                    img.set({ left: x, top: y, scaleX: s, scaleY: s, originX: 'center', originY: 'center' });
                    canvas.add(img); canvas.setActiveObject(img);
                });
            };
            r.readAsDataURL(f);
            e.target.value = '';
        });
    }

    bindUpload('file-hood', 512, 150); bindUpload('file-left', 150, 512);
    bindUpload('file-right', 870, 512); bindUpload('file-rear', 512, 850);
    window.onload = function() { resizeCanvas(); changeModel(); }

    // 1. 点击生成：只负责 Fetch 数据并展示 Modal
    function generatePreview() {
        const loading = document.getElementById('loading-overlay');
        loading.style.display = 'flex';

        // 截图
        const oz = canvas.getZoom(), ow = canvas.getWidth(), oh = canvas.getHeight(), obg = canvas.backgroundImage;
        canvas.setZoom(1); canvas.setWidth(1024); canvas.setHeight(1024); canvas.setBackgroundImage(null, canvas.renderAll.bind(canvas));
        const dataURL = canvas.toDataURL({ format: 'png', multiplier: 1 });
        canvas.setZoom(oz); canvas.setWidth(ow); canvas.setHeight(oh); canvas.setBackgroundImage(obg, canvas.renderAll.bind(canvas));

        const formData = new FormData();
        formData.append('image_data', dataURL);
        formData.append('model_id', document.getElementById('model-select').value);

        fetch('/process', { method: 'POST', body: formData })
        .then(res => {
            if(!res.ok) throw new Error("Server error");
            return res.blob();
        })
        .then(blob => {
            // 生成 Blob URL
            if(currentBlobURL) URL.revokeObjectURL(currentBlobURL);
            currentBlobURL = URL.createObjectURL(blob);
            
            // 显示预览
            document.getElementById('preview-img').src = currentBlobURL;
            document.getElementById('preview-modal').style.display = 'flex';
        })
        .catch(err => alert("Error: " + err))
        .finally(() => loading.style.display = 'none');
    }

    // 2. 点击保存：触发浏览器下载
    function downloadImage() {
        const a = document.createElement('a');
        a.style.display = 'none';
        a.href = currentBlobURL;
        
        let fname = document.getElementById('file-name').value || 'tesla_wrap';
        if(!fname.toLowerCase().endsWith('.png')) fname += '.png';
        a.download = fname;

        document.body.appendChild(a);
        a.click();
        
        // 保存后不强制关闭 Modal，用户可能想多存一份
        // closeModal(); 
    }

    function closePreview() {
        document.getElementById('preview-modal').style.display = 'none';
    }
</script>
</body>
</html>
`
