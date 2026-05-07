import React, { useState, useEffect, useRef, useCallback } from 'react'
import './App.css'

const API_BASE = ''

const VIEW_LABELS = {
  front: '前视图',
  rear: '后视图',
  left: '左视图',
  right: '右视图',
  top: '顶视图',
}

const VIEW_COLORS = {
  front: '#4A90D9',
  rear: '#E74C3C',
  left: '#2ECC71',
  right: '#F39C12',
  top: '#9B59B6',
}

export default function App() {
  const [models, setModels] = useState([])
  const [selectedModel, setSelectedModel] = useState('')
  const [modelDetails, setModelDetails] = useState(null)
  const [images, setImages] = useState({})
  const [adjustments, setAdjustments] = useState({})
  const [activeAdjust, setActiveAdjust] = useState(null)
  const [templateUrl, setTemplateUrl] = useState('')
  const [rendering, setRendering] = useState(false)
  const [resultUrl, setResultUrl] = useState(null)
  const [dragOver, setDragOver] = useState(null)
  const [error, setError] = useState('')
  const [panel, setPanel] = useState('upload') // 'upload' | 'adjust'
  const previewRef = useRef(null)

  useEffect(() => {
    fetch(`${API_BASE}/api/models`)
      .then(r => r.json())
      .then(data => {
        setModels(data)
        if (data.length > 0) setSelectedModel(data[0].id)
      })
      .catch(e => setError('加载车型列表失败: ' + e.message))
  }, [])

  useEffect(() => {
    if (!selectedModel) return
    setResultUrl(null)
    setTemplateUrl(`${API_BASE}/api/models/${selectedModel}/template`)
    fetch(`${API_BASE}/api/models/${selectedModel}`)
      .then(r => r.json())
      .then(data => setModelDetails(data))
      .catch(e => setError('加载车型详情失败: ' + e.message))
    setAdjustments({})
    setActiveAdjust(null)
  }, [selectedModel])

  const handleImageUpload = useCallback((viewName, file) => {
    setImages(prev => ({ ...prev, [viewName]: file }))
    setResultUrl(null)
    // Initialize adjustment for this view
    setAdjustments(prev => ({
      ...prev,
      [viewName]: prev[viewName] || { scale: 100, rotate: 0, offsetX: 0, offsetY: 0, flipH: false },
    }))
  }, [])

  const updateAdjustment = useCallback((viewName, field, value) => {
    setAdjustments(prev => ({
      ...prev,
      [viewName]: { ...prev[viewName], [field]: value },
    }))
    setResultUrl(null)
  }, [])

  const handleDrop = useCallback((e, viewName) => {
    e.preventDefault()
    setDragOver(null)
    const file = e.dataTransfer.files[0]
    if (file && file.type.startsWith('image/')) handleImageUpload(viewName, file)
  }, [handleImageUpload])

  const handleFileInput = useCallback((e, viewName) => {
    const file = e.target.files[0]
    if (file) handleImageUpload(viewName, file)
  }, [handleImageUpload])

  const handleRender = async () => {
    if (!selectedModel || Object.keys(images).length === 0) {
      setError('请先选择车型并上传至少一张图片')
      return
    }
    setRendering(true)
    setError('')
    setResultUrl(null)

    const formData = new FormData()
    formData.append('model_id', selectedModel)
    for (const [view, file] of Object.entries(images)) {
      formData.append(view, file)
    }
    // Send adjustments as JSON
    formData.append('adjustments', JSON.stringify(adjustments))

    try {
      const res = await fetch(`${API_BASE}/api/render`, {
        method: 'POST',
        body: formData,
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text)
      }
      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      setResultUrl(url)
    } catch (e) {
      setError('渲染失败: ' + e.message)
    } finally {
      setRendering(false)
    }
  }

  const handleDownload = () => {
    if (!resultUrl) return
    const a = document.createElement('a')
    a.href = resultUrl
    a.download = `${selectedModel}_wrap.png`
    a.click()
  }

  const handleClearView = (viewName) => {
    setImages(prev => {
      const next = { ...prev }
      delete next[viewName]
      return next
    })
    setAdjustments(prev => {
      const next = { ...prev }
      delete next[viewName]
      return next
    })
  }

  return (
    <div className="app">
      <header className="header">
        <div className="header-content">
          <h1><span className="logo">🚗</span> Tesla Wrap Studio</h1>
          <p className="subtitle">自定义特斯拉车身贴膜设计工具</p>
          <div className="header-tabs">
            <button className={`tab ${panel === 'upload' ? 'active' : ''}`} onClick={() => setPanel('upload')}>
              📤 上传图片
            </button>
            <button className={`tab ${panel === 'adjust' ? 'active' : ''}`} onClick={() => setPanel('adjust')}>
              🎛️ 调整视图
            </button>
          </div>
        </div>
      </header>

      <main className="main">
        {error && (
          <div className="error-banner">
            <span>⚠️ {error}</span>
            <button onClick={() => setError('')}>✕</button>
          </div>
        )}

        <section className="section">
          <label className="section-label">选择车型</label>
          <select className="model-select"
            value={selectedModel}
            onChange={e => setSelectedModel(e.target.value)}>
            {models.map(m => (
              <option key={m.id} value={m.id}>{m.name}</option>
            ))}
          </select>
        </section>

        <div className="workspace">
          {/* Left panel: upload or adjust */}
          <section className="section upload-panel">
            {panel === 'upload' ? (
              <>
                <label className="section-label">上传图片（点击或拖拽）</label>
                <div className="upload-grid">
                  {(modelDetails ? modelDetails.views : []).map(view => {
                    const adj = adjustments[view.name] || { scale: 100, rotate: 0, offsetX: 0, offsetY: 0, flipH: false }
                    return (
                      <UploadBox
                        key={view.name}
                        viewName={view.name}
                        label={VIEW_LABELS[view.name] || view.name}
                        color={VIEW_COLORS[view.name] || '#666'}
                        file={images[view.name]}
                        onUpload={handleImageUpload}
                        onDrop={handleDrop}
                        onClear={() => handleClearView(view.name)}
                        dragOver={dragOver}
                        setDragOver={setDragOver}
                        adjustments={adj}
                      />
                    )
                  })}
                </div>
              </>
            ) : (
              <>
                <label className="section-label">调整视图位置与效果</label>
                <div className="adjust-list">
                  {(modelDetails ? modelDetails.views : []).map(view => {
                    const file = images[view.name]
                    if (!file) return null
                    const adj = adjustments[view.name] || { scale: 100, rotate: 0, offsetX: 0, offsetY: 0, flipH: false }
                    const isActive = activeAdjust === view.name
                    return (
                      <div key={view.name} className={`adjust-item ${isActive ? 'active' : ''}`}
                        onClick={() => setActiveAdjust(view.name)}>
                        <div className="adjust-header">
                          <span className="adjust-label" style={{ background: VIEW_COLORS[view.name] }}>
                            {VIEW_LABELS[view.name]}
                          </span>
                          <div className="adjust-preview-thumb">
                            <img src={URL.createObjectURL(file)} alt="" />
                          </div>
                        </div>
                        {isActive && (
                          <div className="adjust-controls" onClick={e => e.stopPropagation()}>
                            <div className="adjust-row">
                              <span className="adjust-row-label">缩放</span>
                              <input type="range" min="30" max="200"
                                value={adj.scale || 100}
                                onChange={e => updateAdjustment(view.name, 'scale', parseInt(e.target.value))} />
                              <span className="adjust-value">{adj.scale || 100}%</span>
                            </div>
                            <div className="adjust-row">
                              <span className="adjust-row-label">旋转</span>
                              <input type="range" min="-180" max="180"
                                value={adj.rotate || 0}
                                onChange={e => updateAdjustment(view.name, 'rotate', parseInt(e.target.value))} />
                              <span className="adjust-value">{adj.rotate || 0}°</span>
                            </div>
                            <div className="adjust-row">
                              <span className="adjust-row-label">水平偏移</span>
                              <input type="range" min="-50" max="50"
                                value={adj.offsetX || 0}
                                onChange={e => updateAdjustment(view.name, 'offsetX', parseInt(e.target.value))} />
                              <span className="adjust-value">{adj.offsetX || 0}px</span>
                            </div>
                            <div className="adjust-row">
                              <span className="adjust-row-label">垂直偏移</span>
                              <input type="range" min="-50" max="50"
                                value={adj.offsetY || 0}
                                onChange={e => updateAdjustment(view.name, 'offsetY', parseInt(e.target.value))} />
                              <span className="adjust-value">{adj.offsetY || 0}px</span>
                            </div>
                            <div className="adjust-row">
                              <span className="adjust-row-label">水平翻转</span>
                              <label className="toggle-switch">
                                <input type="checkbox"
                                  checked={adj.flipH || false}
                                  onChange={e => updateAdjustment(view.name, 'flipH', e.target.checked)} />
                                <span className="toggle-slider"></span>
                              </label>
                              <span className="adjust-value">{adj.flipH ? '已翻转' : '正常'}</span>
                            </div>
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              </>
            )}
          </section>

          {/* Right panel: preview */}
          <section className="section preview-panel">
            <label className="section-label">预览与结果</label>
            <div className="template-preview" ref={previewRef}>
              {resultUrl ? (
                <img src={resultUrl} alt="渲染结果" className="render-result" />
              ) : (
                <>
                  <img src={templateUrl} alt="模板" className="template-image" />
                  <div className="view-overlay">
                    {modelDetails && modelDetails.views.map(view => {
                      const file = images[view.name]
                      if (!file) return null
                      const imgUrl = URL.createObjectURL(file)
                      const adj = adjustments[view.name] || { scale: 100, rotate: 0, offsetX: 0, offsetY: 0, flipH: false }
                      return (
                        <div key={view.name}
                          className="view-overlay-region"
                          style={{
                            left: `${(view.x / modelDetails.width) * 100}%`,
                            top: `${(view.y / modelDetails.height) * 100}%`,
                            width: `${(view.w / modelDetails.width) * 100}%`,
                            height: `${(view.h / modelDetails.height) * 100}%`,
                            borderColor: VIEW_COLORS[view.name],
                          }}>
                          <div className="overlay-image-wrapper"
                            style={{
                              transform: `scale(${adj.scale / 100}) rotate(${adj.rotate || 0}deg) translate(${adj.offsetX || 0}px, ${adj.offsetY || 0}px)`,
                              transformOrigin: 'center center',
                            }}>
                            <img src={imgUrl} alt={view.name}
                              className="overlay-image"
                              style={{
                                width: '100%', height: '100%',
                                objectFit: 'cover', opacity: 0.7,
                                transform: adj.flipH ? 'scaleX(-1)' : 'none',
                              }}
                            />
                          </div>
                          <div className="region-label" style={{ background: VIEW_COLORS[view.name] }}>
                            {VIEW_LABELS[view.name]}
                          </div>
                        </div>
                      )
                    })}
                  </div>
                </>
              )}
            </div>

            <div className="actions">
              <button className="btn btn-primary"
                onClick={handleRender}
                disabled={rendering || Object.keys(images).length === 0}>
                {rendering ? '⏳ 渲染中...' : '🎨 生成 Wrap'}
              </button>
              {resultUrl && (
                <button className="btn btn-success" onClick={handleDownload}>
                  ⬇️ 下载 PNG
                </button>
              )}
            </div>
            {modelDetails && (
              <div className="model-info">
                <small>模板: {modelDetails.width}×{modelDetails.height} | 视图: {modelDetails.views_count}个 | 已上传: {Object.keys(images).length}/{modelDetails.views_count}</small>
              </div>
            )}
          </section>
        </div>
      </main>
    </div>
  )
}

function UploadBox({ viewName, label, color, file, onUpload, onDrop, onClear, dragOver, setDragOver, adjustments }) {
  const inputRef = useRef(null)
  const previewUrl = file ? URL.createObjectURL(file) : null

  return (
    <div className={`upload-box ${dragOver === viewName ? 'drag-over' : ''} ${file ? 'has-file' : ''}`}
      style={{ borderColor: color }}
      onDragOver={e => { e.preventDefault(); setDragOver(viewName) }}
      onDragLeave={() => setDragOver(null)}
      onDrop={e => onDrop(e, viewName)}
      onClick={() => !file && inputRef.current?.click()}>
      <input ref={inputRef} type="file" accept="image/*"
        style={{ display: 'none' }}
        onChange={e => onUpload(viewName, e.target.files[0])} />
      <div className="upload-label" style={{ background: color }}>{label}</div>
      {previewUrl ? (
        <div className="upload-preview">
          <img src={previewUrl} alt={label}
            style={{
              transform: adjustments?.flipH ? 'scaleX(-1)' : 'none',
            }} />
          <button className="upload-remove" onClick={e => { e.stopPropagation(); onClear() }}>✕</button>
          <div className="upload-badge">✓</div>
          {adjustments && (adjustments.scale !== 100 || adjustments.rotate !== 0) && (
            <div className="upload-adjust-badge">
              {adjustments.scale}% {adjustments.rotate ? `${adjustments.rotate}°` : ''}
            </div>
          )}
        </div>
      ) : (
        <div className="upload-placeholder">
          <span className="upload-icon">📷</span>
          <span className="upload-text">点击或拖拽上传</span>
        </div>
      )}
    </div>
  )
}
