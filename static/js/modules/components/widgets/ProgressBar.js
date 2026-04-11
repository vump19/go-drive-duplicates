import { Component } from '../base/Component.js';

/**
 * 진행률 표시 컴포넌트
 */
export class ProgressBar extends Component {
    constructor(element, options = {}) {
        super(element);
        
        this.options = {
            min: 0,
            max: 100,
            value: 0,
            animated: true,
            striped: false,
            showText: true,
            showPercentage: true,
            color: 'primary',
            height: 'normal', // small, normal, large
            ...options
        };
        
        this.currentValue = this.options.value;
        this.targetValue = this.options.value;
        this.isAnimating = false;
    }

    onInit() {
        this.render();
    }

    template() {
        const percentage = this.getPercentage();
        const heightClass = `progress-${this.options.height}`;
        const colorClass = `progress-${this.options.color}`;
        const stripedClass = this.options.striped ? 'progress-striped' : '';
        const animatedClass = this.options.animated ? 'progress-animated' : '';
        
        return `
            <div class="progress-container ${heightClass}">
                ${this.options.showText ? this.renderText() : ''}
                <div class="progress-track">
                    <div class="progress-fill ${colorClass} ${stripedClass} ${animatedClass}" 
                         style="width: ${percentage}%"
                         role="progressbar" 
                         aria-valuenow="${this.currentValue}" 
                         aria-valuemin="${this.options.min}" 
                         aria-valuemax="${this.options.max}">
                        ${this.options.showPercentage ? `<span class="progress-percentage">${Math.round(percentage)}%</span>` : ''}
                    </div>
                </div>
                ${this.renderStatus()}
            </div>
        `;
    }

    renderText() {
        return `
            <div class="progress-header">
                <span class="progress-label">${this.options.label || ''}</span>
                <span class="progress-value">${this.currentValue}/${this.options.max}</span>
            </div>
        `;
    }

    renderStatus() {
        return `
            <div class="progress-status">
                <span class="progress-status-text"></span>
                <span class="progress-eta"></span>
            </div>
        `;
    }

    getPercentage() {
        const range = this.options.max - this.options.min;
        const value = this.currentValue - this.options.min;
        return Math.max(0, Math.min(100, (value / range) * 100));
    }

    /**
     * 값 설정 (애니메이션 포함)
     * @param {number} value - 새로운 값
     * @param {Object} options - 애니메이션 옵션
     */
    setValue(value, options = {}) {
        const clampedValue = Math.max(this.options.min, Math.min(this.options.max, value));
        this.targetValue = clampedValue;

        if (options.animate !== false && this.options.animated && this.element) {
            this.animateToValue(clampedValue, options);
        } else {
            this.currentValue = clampedValue;
            this.updateDisplay();
        }
    }

    /**
     * 애니메이션으로 값 변경
     * @param {number} targetValue - 목표 값
     * @param {Object} options - 애니메이션 옵션
     */
    animateToValue(targetValue, options = {}) {
        if (this.isAnimating) {
            return;
        }

        const duration = options.duration || 300;
        const easing = options.easing || 'easeOutCubic';
        const startValue = this.currentValue;
        const startTime = Date.now();

        this.isAnimating = true;

        const animate = () => {
            const elapsed = Date.now() - startTime;
            const progress = Math.min(elapsed / duration, 1);
            
            // 이징 함수 적용
            const easedProgress = this.applyEasing(progress, easing);
            
            this.currentValue = startValue + (targetValue - startValue) * easedProgress;
            this.updateDisplay();

            if (progress < 1) {
                requestAnimationFrame(animate);
            } else {
                this.currentValue = targetValue;
                this.isAnimating = false;
                this.updateDisplay();
                
                // 완료 이벤트 발생
                this.emit('progress:complete', {
                    value: this.currentValue,
                    percentage: this.getPercentage()
                });
            }
        };

        requestAnimationFrame(animate);
    }

    /**
     * 이징 함수 적용
     * @param {number} t - 진행률 (0-1)
     * @param {string} easing - 이징 타입
     */
    applyEasing(t, easing) {
        switch (easing) {
            case 'linear':
                return t;
            case 'easeInQuad':
                return t * t;
            case 'easeOutQuad':
                return t * (2 - t);
            case 'easeInOutQuad':
                return t < 0.5 ? 2 * t * t : -1 + (4 - 2 * t) * t;
            case 'easeOutCubic':
                return (--t) * t * t + 1;
            default:
                return t;
        }
    }

    /**
     * 디스플레이 업데이트
     */
    updateDisplay() {
        if (!this.element) return;

        const percentage = this.getPercentage();
        const progressFill = this.$('.progress-fill');
        const progressValue = this.$('.progress-value');
        const progressPercentage = this.$('.progress-percentage');

        if (progressFill) {
            progressFill.style.width = `${percentage}%`;
            progressFill.setAttribute('aria-valuenow', this.currentValue);
        }

        if (progressValue) {
            progressValue.textContent = `${Math.round(this.currentValue)}/${this.options.max}`;
        }

        if (progressPercentage) {
            progressPercentage.textContent = `${Math.round(percentage)}%`;
        }

        // 진행률에 따른 색상 변경
        this.updateColor();
    }

    /**
     * 진행률에 따른 색상 업데이트
     */
    updateColor() {
        const percentage = this.getPercentage();
        const progressFill = this.$('.progress-fill');
        
        if (!progressFill) return;

        // 기존 색상 클래스 제거
        progressFill.classList.remove('progress-success', 'progress-warning', 'progress-danger');

        // 진행률에 따른 색상 적용
        if (percentage >= 100) {
            progressFill.classList.add('progress-success');
        } else if (percentage >= 75) {
            progressFill.classList.add('progress-primary');
        } else if (percentage >= 50) {
            progressFill.classList.add('progress-info');
        } else if (percentage >= 25) {
            progressFill.classList.add('progress-warning');
        } else {
            progressFill.classList.add('progress-danger');
        }
    }

    /**
     * 상태 텍스트 설정
     * @param {string} text - 상태 텍스트
     */
    setStatusText(text) {
        const statusElement = this.$('.progress-status-text');
        if (statusElement) {
            statusElement.textContent = text;
        }
    }

    /**
     * 예상 완료 시간 설정
     * @param {string|number} eta - 예상 완료 시간 (초 또는 텍스트)
     */
    setETA(eta) {
        const etaElement = this.$('.progress-eta');
        if (!etaElement) return;

        if (typeof eta === 'number') {
            if (eta > 0) {
                const minutes = Math.floor(eta / 60);
                const seconds = Math.floor(eta % 60);
                etaElement.textContent = `약 ${minutes > 0 ? `${minutes}분 ` : ''}${seconds}초 남음`;
            } else {
                etaElement.textContent = '';
            }
        } else {
            etaElement.textContent = eta || '';
        }
    }

    /**
     * 라벨 설정
     * @param {string} label - 라벨 텍스트
     */
    setLabel(label) {
        this.options.label = label;
        const labelElement = this.$('.progress-label');
        if (labelElement) {
            labelElement.textContent = label;
        }
    }

    /**
     * 옵션 업데이트
     * @param {Object} newOptions - 새로운 옵션
     */
    updateOptions(newOptions) {
        this.options = { ...this.options, ...newOptions };
        this.render();
    }

    /**
     * 진행률 리셋
     */
    reset() {
        this.setValue(this.options.min, { animate: false });
        this.setStatusText('');
        this.setETA('');
    }

    /**
     * 완료 상태로 설정
     */
    complete() {
        this.setValue(this.options.max);
        this.setStatusText('완료');
        this.setETA('');
    }

    /**
     * 오류 상태로 설정
     * @param {string} message - 오류 메시지
     */
    error(message = '오류 발생') {
        this.setStatusText(message);
        this.setETA('');
        
        const progressFill = this.$('.progress-fill');
        if (progressFill) {
            progressFill.classList.add('progress-error');
        }
    }

    /**
     * 무한 진행률 모드 설정
     * @param {boolean} enabled - 무한 모드 활성화
     */
    setIndeterminate(enabled) {
        const progressFill = this.$('.progress-fill');
        if (progressFill) {
            if (enabled) {
                progressFill.classList.add('progress-indeterminate');
            } else {
                progressFill.classList.remove('progress-indeterminate');
            }
        }
    }

    /**
     * 현재 값 반환
     */
    getValue() {
        return this.currentValue;
    }

    /**
     * 현재 진행률(%) 반환
     */
    getPercentageValue() {
        return this.getPercentage();
    }

    /**
     * 완료 여부 확인
     */
    isComplete() {
        return this.currentValue >= this.options.max;
    }
}