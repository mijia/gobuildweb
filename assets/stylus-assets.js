/**
 * Created by lengbing on 15/9/15.
 */
/**
 * Stylus Plugin for use asset_map in *.styl
 */

var stylus = require('stylus'),
    path   = require('path'),
    fs     = require('fs'),
    crypto = require('crypto'),
    nodes = stylus.nodes;


// `process.cwd()` is the project root path here

//var jsonFile = path.join(process.cwd(), './assets_map.json');
//var list = require(jsonFile);

function md5(filepath, algorithm, encoding, fileEncoding) {
    var hash = crypto.createHash(algorithm);
    hash.update(fs.readFileSync(filepath), fileEncoding);
    return hash.digest(encoding);
}

function getFileHashName(filename){
    var filepath  = path.join(process.cwd(), 'assets', filename);
    var name      = path.basename(filename);
    var fileStats = fs.statSync(filepath);

    if(fileStats.isFile()){
        var hash = md5(filepath, 'md5', 'hex', 'utf8');
        var result =path.join(path.dirname(filename), 'fp'+ hash + '-' + name);
        return result;
    }else {
        return filename;
    }
}

/**
 * add custom functions
 * @returns {Function}
 */
function plugin(){
    return function(style){
        style.define('assets', assets);
        style.define('images', images);
    }
}

/**
 * get the real file name from the json file
 *
 * @example
 *      assets("images/a/b.jpg") => url("../images/a/fpXXX-b.jpg")
 *
 * @param file  the original file path in assets directory
 * @returns {stylus.nodes.liter}
 */
function assets(file){
    var liter;
    var filepath = file.val;
    filepath = filepath.replace(/^\.\.\//, '');
    liter = getFileHashName(filepath);
    return  new nodes.Literal('url("../'+ liter +'")')
}

/**
 * short for assets call start with "images/"
 *
 * @example
 *      images("a/b.jpg") => url("../images/a/fpXXX-b.jpg")
 *
 * @param file the file path in assets/images directory
 * @returns {stylus.nodes.liter}
 *
 * 简写 assets("images/**");
 * @param 文件夹`images`后边的文件名
 * @returns {stylus.nodes.liter}
 */
function images(file){
    //console.log(path.join('images/', file.val));
    
    // 这里传入一个假的{val},结构, 兼容默认的字符串节点接口
    // call on a fack {val} stuct for `assets` use it as a real string Node
    return assets({
        val: path.join('images/', file.val)
    });
}


exports = module.exports = plugin;
